package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"kafka.local/quotes-api/models"
)

// TokenHandler : Access to token requests collection
type TokenHandler struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewTokenHandler : Returns a new Token Handler
func NewTokenHandler(ctx context.Context, collection *mongo.Collection) *TokenHandler {
	return &TokenHandler{collection: collection, ctx: ctx}
}

// decodeEmailParam : normalizes the :email path parameter.
//
// SECURITY (Audit C-9): the previous decode replaced the FIRST '+' with '@'.
// Combined with the web tier's old '@'->'+' encoding, a plus-addressed email
// like "user+tag@gmail.com" arrived as "user+tag+gmail.com" and was decoded
// to "user@tag+gmail.com" — a corrupted address that was then stored and
// mailed. The web tier now sends the address verbatim (percent-encoded), so:
//
//   - If the parameter already contains '@', it is a properly transmitted
//     address: use it as-is.
//   - Otherwise, assume the LEGACY '+' encoding and restore the '@' at the
//     LAST '+' (the encoded '@' is always the final one, since the local
//     part may itself contain '+' but the domain cannot).
func decodeEmailParam(raw string) string {
	if strings.Contains(raw, "@") {
		return raw
	}
	if i := strings.LastIndex(raw, "+"); i != -1 {
		return raw[:i] + "@" + raw[i+1:]
	}
	return raw
}

// EmailExists : check to see if an email is already added to the token database
//
// NOTE (Audit C-9, accepted deviation): this GET endpoint intentionally
// inserts the record when it is absent — the insert IS the atomic
// existence-check (arbitrated by the unique index on email, surfacing as
// IsDuplicateKeyError below). Converting to POST is tracked in the
// remediation ledger as a future contract change requiring coordinated
// client updates.
func (handler *TokenHandler) EmailExists(c *gin.Context) {

	tokenRequest := models.TokenRequest{
		ID:      primitive.NewObjectID(),
		Email:   decodeEmailParam(c.Param("email")),
		Granted: "false",
	}

	_, err := handler.collection.InsertOne(c.Request.Context(), tokenRequest)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(
				http.StatusOK,
				models.TokenQueryResponse{
					Result: 1,
				})

			return
		}

		c.JSON(
			http.StatusInternalServerError,
			models.TokenQueryResponse{
				Result: 500,
			})

		return
	}

	c.JSON(
		http.StatusOK,
		models.TokenQueryResponse{
			Result: 0,
		})

	return

}

// FetchTokenRequests : fetch all token request records
func (handler *TokenHandler) FetchTokenRequests(c *gin.Context) {
	ctx := c.Request.Context()
	curs, err := handler.collection.Find(ctx, bson.D{{}})
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	defer curs.Close(ctx)

	// O-2: curs.All surfaces decode errors the manual loop discarded.
	tokenRequestRecords := make([]models.TokenRequest, 0)
	if err := curs.All(ctx, &tokenRequestRecords); err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	if len(tokenRequestRecords) == 0 {
		c.JSON(
			http.StatusOK,
			models.NotFoundErrorResponse{
				Status: http.StatusOK,
				Err:    "No Token Request Records",
				Query:  "All Records",
			})

		return
	}

	c.JSON(http.StatusOK, tokenRequestRecords)

}

// AddTokenRequest : add a token request
func (handler *TokenHandler) AddTokenRequest(c *gin.Context) {
	var tokenRequest models.TokenRequest
	if err := c.ShouldBindJSON(&tokenRequest); err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    err.Error(),
			})

		return
	}

	tokenRequest.ID = primitive.NewObjectID()
	_, err := handler.collection.InsertOne(c.Request.Context(), tokenRequest)
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	c.JSON(
		http.StatusOK,
		models.TokenRequestSuccessResponse{
			Status: http.StatusOK,
			Data:   tokenRequest,
		})

}

// UpdateTokenRequest : update the token request record
func (handler *TokenHandler) UpdateTokenRequest(c *gin.Context) {
	id := c.Param("id")

	var tokenRequest models.TokenRequest
	if err := c.ShouldBindJSON(&tokenRequest); err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    err.Error(),
			})

		return
	}

	// O-2: reject malformed IDs instead of updating the zero ObjectID.
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    "invalid id format",
			})

		return
	}

	// O-2: dropped the redundant {"id", id} field (identity is _id).
	res, err := handler.collection.UpdateOne(c.Request.Context(), bson.M{
		"_id": objectID,
	}, bson.D{{"$set", bson.D{
		{"email", tokenRequest.Email},
		{"granted", tokenRequest.Granted},
	}}})

	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	// O-2: no match → 404 rather than a silent success.
	if res.MatchedCount == 0 {
		c.JSON(
			http.StatusNotFound,
			models.NotFoundErrorResponse{
				Status: http.StatusNotFound,
				Err:    "no token request with that id",
				Query:  id,
			})

		return
	}

	// O-3: the handler previously fell off the end with no body, returning an
	// empty 200. Emit an explicit success response mirroring the other
	// mutating token handlers.
	tokenRequest.ID = objectID
	c.JSON(
		http.StatusOK,
		models.TokenRequestSuccessResponse{
			Status: http.StatusOK,
			Data:   tokenRequest,
		})
}

// DeleteTokenRequest : remove a token request record
func (handler *TokenHandler) DeleteTokenRequest(c *gin.Context) {
	id := c.Param("id")

	// O-2: reject malformed IDs instead of deleting against the zero ObjectID.
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    "invalid id format",
			})

		return
	}

	res, err := handler.collection.DeleteOne(c.Request.Context(), bson.M{"_id": objectID})
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	// O-2: nothing deleted → 404 rather than reporting "deleted".
	if res.DeletedCount == 0 {
		c.JSON(
			http.StatusNotFound,
			models.NotFoundErrorResponse{
				Status: http.StatusNotFound,
				Err:    "no token request with that id",
				Query:  id,
			})

		return
	}

	c.JSON(
		http.StatusOK,
		models.DeleteResponse{
			Status:   http.StatusOK,
			ID:       id,
			Response: "deleted",
		})
}
