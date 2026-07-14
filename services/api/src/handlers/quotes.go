package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"kafka.local/quotes-api/dbs"
	"kafka.local/quotes-api/models"
)

// searchResultLimit caps how many documents a single text search may return
// (Audit P-1). Without a cap, a broad query ("the", "a") serialized the entire
// collection into memory. 200 is generous for a UI result list while keeping
// per-request allocation bounded.
const searchResultLimit = 200

// QuoteHandler : handler router
type QuoteHandler struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewQuoteHandler : returns a new quote handler
func NewQuoteHandler(ctx context.Context, collection *mongo.Collection) *QuoteHandler {
	return &QuoteHandler{collection: collection, ctx: ctx}
}

// SearchQuotesHandler : search for quotes
// @summary      search for quotes by keywords
// @description  search for quotes using a comma separated string of keywords
// @produce      json
// @param        keywords  query     string  true                  "word1,word2,word3"
// @success      200       {object}  []models.Quote                "list of quotes"
// @failure      500       {object}  models.ErrorResponse          "server error"
// @failure      404       {object}  models.NotFoundErrorResponse  "no matches found"
// @router       /quote/search [post]
func (handler *QuoteHandler) SearchQuotesHandler(c *gin.Context) {
	var query models.Query
	if err := c.ShouldBindJSON(&query); err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    err.Error(),
			})

		return
	}

	// PERFORMANCE (Audit P-1): the previous implementation ran one $text query
	// PER phrase in a sequential loop plus another for terms (N+1 round-trips),
	// concatenated the results (so a quote matching both a phrase and a term
	// was returned twice), silently discarded decode errors, and applied no
	// limit (a broad search buffered the whole collection into memory).
	//
	// MongoDB's $text $search string natively supports mixing quoted phrases
	// and bare terms in a SINGLE expression, e.g.  `"carpe diem" fortune fate`.
	// We build that one string and issue exactly one bounded query.
	var sb strings.Builder
	for _, phrase := range query.Phrases {
		// %q wraps the phrase in double quotes and escapes any embedded quotes,
		// which is exactly the phrase syntax $text expects.
		fmt.Fprintf(&sb, "%q ", phrase)
	}
	sb.WriteString(strings.Join(query.Terms, " "))
	search := strings.TrimSpace(sb.String())

	if search == "" {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    "no search terms or phrases provided",
			})

		return
	}

	ctx := c.Request.Context()
	curs, err := handler.collection.Find(
		ctx,
		bson.D{{"$text", bson.D{{"$search", search}}}},
		options.Find().SetLimit(searchResultLimit), // bounded memory (P-1)
	)
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

	// curs.All surfaces decode errors (previously swallowed) and dedupe is
	// implicit — a single query cannot return the same document twice.
	quotes := make([]models.Quote, 0)
	if err := curs.All(ctx, &quotes); err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	if len(quotes) == 0 {
		c.JSON(
			http.StatusNotFound,
			models.QueryNotFoundErrorResponse{
				Status:  http.StatusNotFound,
				Err:     "no data found",
				Queries: query,
			})

		return
	}

	c.JSON(http.StatusOK, quotes)
}

// RandomQuoteHandler : fetch a single random quote
// @summary      fetch a random quote
// @description  a call to this endpoint fetches a single random quote as JSON
// @produce      json
// @success      200  {object}    models.Quote          "quote JSON"
// @failure      500  {object}    models.ErrorResponse  "server error"
// @router       /quote [get]
func (handler *QuoteHandler) RandomQuoteHandler(c *gin.Context) {
	// PERFORMANCE / CORRECTNESS (Audit P-4): the previous version sampled TWO
	// documents and used a bizarre nested `for curs.Next { for curs.Next {`
	// loop that discarded the first, returned the second, and ignored decode
	// errors — it only worked by accident and fetched 2x the data. We now
	// sample exactly one document and decode it via curs.All (which surfaces
	// errors). The wire shape is unchanged: an array containing one quote.
	ctx := c.Request.Context()
	curs, err := handler.collection.Aggregate(
		ctx,
		mongo.Pipeline{{{"$sample", bson.M{"size": 1}}}},
	)
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

	quotes := make([]models.Quote, 0, 1)
	if err := curs.All(ctx, &quotes); err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	c.JSON(http.StatusOK, quotes)
}

// GetQuoteHandler : fetch a single quote by id
// @summary      fetch a single quote
// @description  fetch a quote by quote ID
// @produce      json
// @param        id   path        string  true          "quote id"
// @success      200  {object}    models.Quote          "quote JSON"
// @failure      500  {object}    models.ErrorResponse  "server error"
// @router       /quote/{id} [get]
func (handler *QuoteHandler) GetQuoteHandler(c *gin.Context) {
	id := c.Param("id")

	// O-2: the parse error was previously discarded, so "/quote/garbage"
	// silently queried the zero ObjectID. Reject malformed IDs up front.
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

	cur := handler.collection.FindOne(c.Request.Context(), bson.M{"_id": objectID})

	var quote models.Quote
	if err := cur.Decode(&quote); err != nil {
		// O-1: match the driver's sentinel error instead of comparing the
		// human-readable message (which can change between releases).
		if errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(
				http.StatusNotFound,
				models.NotFoundErrorResponse{
					Status: http.StatusNotFound,
					Err:    "no documents found",
					Query:  fmt.Sprintf("Object %s", id),
				})

			return
		}

		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	c.JSON(http.StatusOK, quote)
}

// GetAuthorsHandler : fetch a list of authors
// @summary      fetch a list of authors
// @description  fetch a list of all authors stored in the database
// @produce      json
// @success      200  {object}    models.Authors        "list of authors"
// @failure      500  {object}    models.ErrorResponse  "server error"
// @router       /authors [get]
func (handler *QuoteHandler) GetAuthorsHandler(c *gin.Context) {
	results, err := handler.collection.Distinct(c.Request.Context(), "attribution", bson.D{{}})
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	// O-5: Distinct already returns []interface{}, which is exactly the type
	// of Authors.Names — the previous element-by-element loop was a no-op copy.
	c.JSON(http.StatusOK, models.Authors{Names: results})
}

// GetQuotesByAuthorHandler : fetch all quotes by an author
// @summary      fetch quotes using  author's name
// @description  returns a list of all quotes found by the specificed author
// @produce      json
// @param        name  path       string  true          "author's name"
// @success      200  {object}    models.Author         "list of quotes JSON"
// @failure      500  {object}    models.ErrorResponse  "server error"
// @failure      404  {object}    models.NotFoundErrorResponse  "author not found"
// @router       /authors/{name} [get]
func (handler *QuoteHandler) GetQuotesByAuthorHandler(c *gin.Context) {
	author := c.Param("name")
	name := dbs.CreateRegexQueryString(author)

	ctx := c.Request.Context()
	curs, err := handler.collection.Find(
		ctx,
		bson.M{"attribution": bson.M{"$regex": primitive.Regex{Pattern: name, Options: "i"}}},
	)

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

	// O-2/O-5: curs.All surfaces decode errors that the manual loop discarded.
	quotes := make([]models.Quote, 0)
	if err := curs.All(ctx, &quotes); err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	if len(quotes) == 0 {
		c.JSON(
			http.StatusNotFound,
			models.NotFoundErrorResponse{
				Status: http.StatusNotFound,
				Err:    "author not found",
				Query:  author,
			})

		return
	}

	c.JSON(http.StatusOK, models.Author{Name: author, Quotes: quotes})
}

// AddQuoteHandler : add a quote to the database
// @summary      add a new quote
// @description  add a new quote to the database
// @accept       json
// @produce      json
// @security     ApiKeyAuth
// @param  Authorization header string true "Authorization"
// @param data   body             models.NewQuote true      "JSON"
// @success      200  {object}    models.SuccessResponse    "success response"
// @failure      500  {object}    models.ErrorResponse      "database error"
// @failure      400  {object}    models.ErrorResponse      "bad request"
// @router       /quote [post]
func (handler *QuoteHandler) AddQuoteHandler(c *gin.Context) {
	var quote models.NewQuote
	if err := c.ShouldBindJSON(&quote); err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    err.Error(),
			})

		return
	}

	quote.ID = primitive.NewObjectID()
	quote.UID = fmt.Sprintf("%s", c.MustGet("uid"))
	quote.Tag = dbs.CreateTag(quote.Author, quote.Text)

	_, err := handler.collection.InsertOne(c.Request.Context(), quote)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			dbErr := err.(mongo.WriteException)
			c.JSON(
				http.StatusInternalServerError,
				models.DBErrorResponse{
					Type: "Write Exception",
					Code: dbErr.WriteErrors[0].Code,
					Err:  "similar record exists",
				})

			return
		}

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
		models.PostSuccessResponse{
			Status: http.StatusOK,
			Data:   quote,
		})
}

// UpdateQuoteHandler : updates a recipe in the database
// @summary      replace a quote
// @description  replace quote by id
// @accept       json
// @produce      json
// @security ApiKeyAuth
// @param Authorization header string true "X-API-KEY"
// @param        id    path       string  true              "quote id"
// @param        data  body       models.NewQuote true      "JSON"
// @success      200  {object}    models.SuccessResponse    "success response"
// @failure      500  {object}    models.ErrorResponse      "database error"
// @failure      400  {object}    models.ErrorResponse      "bad request"
// @router       /quote/{id} [put]
func (handler *QuoteHandler) UpdateQuoteHandler(c *gin.Context) {
	id := c.Param("id")

	var quote models.Quote
	if err := c.ShouldBindJSON(&quote); err != nil {
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

	// O-2: dropped the redundant {"id", id} field — document identity is _id;
	// storing a stringified duplicate only pollutes the schema.
	res, err := handler.collection.UpdateOne(c.Request.Context(), bson.M{
		"_id": objectID,
	}, bson.D{{"$set", bson.D{
		{"attribution", quote.Author},
		{"quote", quote.Text},
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

	// O-2: a matched count of zero means no such document — report 404 rather
	// than a misleading "success" for a write that changed nothing.
	if res.MatchedCount == 0 {
		c.JSON(
			http.StatusNotFound,
			models.NotFoundErrorResponse{
				Status: http.StatusNotFound,
				Err:    "no document with that id",
				Query:  id,
			})

		return
	}

	quote.ID = objectID
	c.JSON(
		http.StatusOK,
		models.SuccessResponse{
			Status: http.StatusOK,
			Data:   quote,
		})
}

// DeleteQuoteHandler : deletes a recipe in the database
// @summary      delete a quote
// @description  delete quote by id
// @accept       json
// @produce      json
// @security ApiKeyAuth
// @param Authorization header string true "X-API-KEY"
// @param        id    path       string true               "quote id"
// @success      200  {object}    models.SuccessResponse    "success response"
// @failure      500  {object}    models.ErrorResponse      "database error"
// @failure      400  {object}    models.ErrorResponse      "bad request"
// @router       /quote/{id} [delete]
func (handler *QuoteHandler) DeleteQuoteHandler(c *gin.Context) {
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

	// O-2: nothing deleted → 404, so callers aren't told "deleted" for a
	// document that never existed.
	if res.DeletedCount == 0 {
		c.JSON(
			http.StatusNotFound,
			models.NotFoundErrorResponse{
				Status: http.StatusNotFound,
				Err:    "no document with that id",
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
