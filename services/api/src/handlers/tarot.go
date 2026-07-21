package handlers

import (
	"context"
	"errors"
	"math/rand"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"kafka.local/quotes-api/models"
	"kafka.local/quotes-api/utils"
)

// SECURITY / STABILITY (Audit C-6) constants.
const (
	// maxSpreadPositions caps the number of positions a client may request in
	// a custom spread. Without a cap, a body with tens of thousands of
	// positions triggered one aggregation pipeline PER position in "all"
	// mode — an unauthenticated resource-exhaustion DoS. No physical tarot
	// deck exceeds 78 cards, so 78 is a generous, semantically honest limit.
	maxSpreadPositions = 78
)

// NOTE (Audit C-6): the per-request rand.Seed(time.Now().UnixNano()) calls
// were removed. Since Go 1.20 the global source is automatically seeded, the
// API is deprecated, and re-seeding on every request meant concurrent requests
// landing in the same nanosecond drew IDENTICAL "random" sequences.

// NOTE (2026-07-21): random deck selection previously GUESSED a numeric key
// via rand.Intn over a hardcoded deckCount of 69. The data actually holds 70
// decks numbered 1..72 with gaps at 9 and 43, so ~3% of draws sampled a
// missing key and returned HTTP 500 ("deck sample returned no cards" /
// decode error), compounding per position in "all"-mode spreads — while
// decks 70..72 could never be drawn at all. Selection now samples an ACTUAL
// document with $sample, which follows the data wherever its keys go and
// removed the deckCount constant entirely.

// sampleDeck returns one uniformly random deck document via a $sample
// aggregation. Sampling documents rather than guessing numeric keys keeps
// every existing deck reachable and cannot select a key that has no document.
func (handler *TarotHandler) sampleDeck(ctx context.Context) (models.Deck, error) {
	cursor, err := handler.collection.Aggregate(ctx, bson.A{
		bson.D{{"$sample", bson.D{{"size", 1}}}},
	})
	if err != nil {
		return models.Deck{}, err
	}

	var decks []models.Deck
	if err = cursor.All(ctx, &decks); err != nil {
		return models.Deck{}, err
	}

	// Defensive: an empty collection is the only way $sample yields nothing.
	if len(decks) == 0 {
		return models.Deck{}, errors.New("deck sample returned no documents")
	}

	return decks[0], nil
}

// TarotHandler : handler router
type TarotHandler struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewTarotHandler : returns a new quote handler
func NewTarotHandler(ctx context.Context, collection *mongo.Collection) *TarotHandler {
	return &TarotHandler{collection: collection, ctx: ctx}
}

// TarotSpreadHandler : generates a user-defined Tarot Spread
// @summary      generate a custom Tarot Spread
// @description  post a spread and recieve a reading using that spread
// @produce      json
// @accepts      json
// @success      200       {object}  models.Reading                "tarot reading object"
// @failure      400       {object}  models.ErrorResponse          "bad request"
// @failure      500       {object}  models.ErrorResponse          "server error"
// @router       /tarot/spread [post]
func (handler *TarotHandler) TarotSpreadHandler(c *gin.Context) {
	var spread models.CustomSpread
	if err := c.ShouldBindJSON(&spread); err != nil {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    err.Error(),
			})

		return
	}

	// --- Input bounds (Audit C-6): reject before doing ANY database work. ---
	// An empty spread is meaningless; an oversized one was a DoS vector
	// (N aggregations in "all" mode) and a guaranteed panic in single-deck
	// mode once positions outnumbered the remaining cards.
	if n := len(spread.Positions); n == 0 || n > maxSpreadPositions {
		c.JSON(
			http.StatusBadRequest,
			models.ErrorResponse{
				Status: http.StatusBadRequest,
				Err:    "positions must contain between 1 and 78 entries",
			})

		return
	}

	// Run all DB work under the request context so queries are cancelled if
	// the client disconnects (fail-safe resource release; Audit C-10).
	ctx := c.Request.Context()

	if spread.Deck == "all" {

		layout := make(map[string]string)
		for _, v := range spread.Positions {
			// Stage order preserves the original draw semantics: pick ONE
			// deck uniformly ($sample over documents), then one card
			// uniformly within it ($unwind + $sample). Sampling documents
			// instead of $match-ing a guessed numeric key is the 2026-07-21
			// fix — a guessed key could name a gap and match nothing.
			cursor, err := handler.collection.Aggregate(ctx, bson.A{
				bson.D{{"$sample", bson.D{{"size", 1}}}},
				bson.D{{"$unwind", bson.D{{"path", "$cards"}}}},
				bson.D{{"$sample", bson.D{{"size", 1}}}},
				bson.D{{"$unset", bson.A{"_id", "deck", "name"}}},
			})

			if err != nil {
				c.JSON(
					http.StatusInternalServerError,
					models.ErrorResponse{
						Status: http.StatusInternalServerError,
						Err:    err.Error(),
					})

				return
			}

			var card []models.SingleCard
			if err = cursor.All(ctx, &card); err != nil {
				c.JSON(
					http.StatusInternalServerError,
					models.ErrorResponse{
						Status: http.StatusInternalServerError,
						Err:    err.Error(),
					})

				return
			}

			// PANIC GUARD (Audit C-6): card[0] previously indexed blindly.
			// If the sampled deck number has no document (data gap), the
			// slice is empty and indexing it crashed the handler.
			if len(card) == 0 {
				c.JSON(
					http.StatusInternalServerError,
					models.ErrorResponse{
						Status: http.StatusInternalServerError,
						Err:    "deck sample returned no cards",
					})

				return
			}

			layout[v] = card[0].Card

		}

		c.JSON(
			http.StatusOK,
			models.Reading{
				Name:   spread.Name,
				Layout: layout,
			})

		return

	}

	var deck models.Deck
	var err error

	if spread.Deck == "any" {
		deck, err = handler.sampleDeck(ctx)
	} else {
		err = handler.collection.FindOne(ctx, bson.M{"name": spread.Deck}).Decode(&deck)
	}

	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	layout := make(map[string]string)
	for _, v := range spread.Positions {
		// PANIC GUARD (Audit C-6): drawing without replacement shrinks
		// deck.Cards each iteration. If positions outnumber the cards,
		// rand.Intn(0) panics — reject cleanly instead.
		if len(deck.Cards) == 0 {
			c.JSON(
				http.StatusBadRequest,
				models.ErrorResponse{
					Status: http.StatusBadRequest,
					Err:    "spread has more positions than the selected deck has cards",
				})

			return
		}

		i := rand.Intn(len(deck.Cards))
		layout[v] = deck.Cards[i]
		deck.Cards = utils.RemoveIndex(deck.Cards, i)
	}

	c.JSON(
		http.StatusOK,
		models.Reading{
			Name:   spread.Name,
			Layout: layout,
		})
}

// TarotDeckHandler : fetch an entire Tarot deck
// @summary      fetch an entire tarot deck
// @description  fetch a deck using the deck object id
// @produce      json
// @success      200       {object}  models.Deck                   "list of cards"
// @failure      500       {object}  models.ErrorResponse          "server error"
// @failure      404       {object}  models.NotFoundErrorResponse  "no deck found"
// @router       /tarot/deck/:id [get]
func (handler *TarotHandler) TarotDeckHandler(c *gin.Context) {
	objectID, _ := primitive.ObjectIDFromHex(c.Param("id"))
	cur := handler.collection.FindOne(c.Request.Context(), bson.M{"_id": objectID})

	var deck models.Deck
	err := cur.Decode(&deck)
	if err != nil {
		// O-1: use the driver's sentinel error, not a message string compare.
		if errors.Is(err, mongo.ErrNoDocuments) {
			c.JSON(
				http.StatusNotFound,
				models.TarotNotFoundResponse{
					Status: http.StatusNotFound,
					Err:    "no documents found",
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

	c.JSON(http.StatusOK, deck)
}

// RandomCardHandler : fetch a random Tarot card
// @summary      fetch a random tarot
// @produce      json
// @success      200       {object}  models.Card                   "random card"
// @failure      500       {object}  models.ErrorResponse          "server error"
// @router       /tarot/card [get]
func (handler *TarotHandler) RandomCardHandler(c *gin.Context) {
	deck, err := handler.sampleDeck(c.Request.Context())
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	// PANIC GUARD (Audit C-6): an empty deck previously panicked on indexing,
	// and a one-card deck panicked via rand.Intn(0).
	if len(deck.Cards) == 0 {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    "deck contains no cards",
			})

		return
	}

	card := models.Card{
		ID:   deck.ID,
		Deck: deck.Name,
		// OFF-BY-ONE FIX (Audit C-6): was rand.Intn(len-1), which both
		// panicked on single-card decks and silently made the LAST card
		// undrawable. Intn(len) covers the full range safely.
		Card: deck.Cards[rand.Intn(len(deck.Cards))],
	}

	c.JSON(http.StatusOK, card)

}

// RandomDeckHandler : fetch a random Tarot Deck
// @summary      fetch a random tarot deck
// @produce      json
// @success      200       {object}  models.Deck                   "random deck"
// @failure      500       {object}  models.ErrorResponse          "server error"
// @router       /tarot/deck [get]
func (handler *TarotHandler) RandomDeckHandler(c *gin.Context) {
	deck, err := handler.sampleDeck(c.Request.Context())
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	c.JSON(http.StatusOK, deck)

}

// ListDecksHandler : fetch a random Tarot Deck
// @summary      fetch a list of all availale tarot decks
// @produce      json
// @success      200       {object}  []models.Decks                  "random deck"
// @failure      500       {object}  models.ErrorResponse            "server error"
// @router       /tarot/decks [get]
func (handler *TarotHandler) ListDecksHandler(c *gin.Context) {
	results, err := handler.collection.Distinct(c.Request.Context(), "name", bson.D{{}})
	if err != nil {
		c.JSON(
			http.StatusInternalServerError,
			models.ErrorResponse{
				Status: http.StatusInternalServerError,
				Err:    err.Error(),
			})

		return
	}

	// O-5: Distinct already returns []interface{} (the type of Decks.Names) —
	// the element-by-element loop was a no-op copy.
	c.JSON(http.StatusOK, models.Decks{Names: results})
}
