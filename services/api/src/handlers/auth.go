package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"kafka.local/quotes-api/models"
)

// AuthoHandler : Authorization Middleware
type AuthoHandler struct {
	collection *mongo.Collection
	ctx        context.Context
}

// NewAuthoHandler : Returns a new Auth Handler
func NewAuthoHandler(ctx context.Context, collection *mongo.Collection) *AuthoHandler {
	return &AuthoHandler{collection: collection, ctx: ctx}
}

// AuthMiddleware : Validates the API Key.
//
// SECURITY (Audit C-1): In Gin, returning from a middleware WITHOUT calling
// c.Abort() does NOT stop the handler chain — the engine's dispatch loop simply
// continues to the next handler. The previous implementation had two failure
// modes because of this:
//
//  1. On a failed key lookup it called AbortWithStatus(401) but kept executing,
//     causing a second (500) response to be written on top of the 401.
//  2. On a decode error it wrote a 500 and returned WITHOUT aborting, so the
//     protected handler still ran with an empty user — an authentication bypass.
//
// This version fails closed: every rejection path terminates the chain
// immediately via AbortWithStatusJSON and returns a standardized
// models.ErrorResponse envelope. c.Next() is only reachable after a
// successful credential lookup.
func (handler *AuthoHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Reject requests with no credential up front. Do not touch the
		// database for anonymous traffic.
		key := c.GetHeader("Authorization")
		if key == "" {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				models.ErrorResponse{
					Status: http.StatusUnauthorized,
					Err:    "missing credentials",
				})

			return
		}

		// Single error gate: FindOne + Decode combined so there is exactly one
		// place where lookup/decode failures are handled. The query runs under
		// the REQUEST context (not the boot-time context) so it is cancelled
		// automatically if the client disconnects.
		var user models.User
		err := handler.collection.
			FindOne(c.Request.Context(), bson.M{"authorization": key}).
			Decode(&user)

		if err != nil {
			// Unknown key → 401. Use the driver's sentinel error, never
			// string-compare error messages.
			if errors.Is(err, mongo.ErrNoDocuments) {
				// NOTE: we deliberately do NOT log the presented key — never
				// write credentials (even invalid ones) to logs.
				log.Printf("auth: rejected request to %q: unknown API key", c.Request.URL.Path)
				c.AbortWithStatusJSON(
					http.StatusUnauthorized,
					models.ErrorResponse{
						Status: http.StatusUnauthorized,
						Err:    "invalid credentials",
					})

				return
			}

			// Any other failure (driver error, malformed document, etc.) is a
			// server-side fault. FAIL CLOSED: abort with 500 — do not fall
			// through to the protected handler in a degraded state. The
			// client-facing message is generic; details go to the server log.
			log.Printf("auth: lookup error for %q: %v", c.Request.URL.Path, err)
			c.AbortWithStatusJSON(
				http.StatusInternalServerError,
				models.ErrorResponse{
					Status: http.StatusInternalServerError,
					Err:    "authorization lookup failed",
				})

			return
		}

		// Authentication succeeded — pass the UID to the next middleware.
		c.Set("uid", user.UID)
		c.Next()
	}
}

// AdminAuthMiddleware : validates user id against the configured admin UID.
//
// SECURITY (Audit C-1): The previous implementation compared
// c.MustGet("uid") == os.Getenv("ADMIN_ID") on every request. If ADMIN_ID was
// unset in the environment it evaluated to "", meaning any request whose
// decoded user carried an empty uid would PASS the admin check — a privilege
// escalation under misconfiguration. It also called c.Next() after an abort
// and would panic (via MustGet) if the uid key was absent.
//
// This version:
//   - Reads ADMIN_ID exactly once, at middleware construction.
//   - Fails closed on misconfiguration: an empty ADMIN_ID refuses ALL admin
//     traffic with 500 rather than silently disabling the check.
//   - Uses c.Get (no panic) and aborts + returns on every rejection path.
func (handler *AuthoHandler) AdminAuthMiddleware() gin.HandlerFunc {
	// Read once: the admin UID is fixed for the lifetime of the container.
	adminID := os.Getenv("ADMIN_ID")

	return func(c *gin.Context) {
		// FAIL CLOSED: a missing ADMIN_ID is an operator error, not an
		// invitation to skip authorization. Refuse everything until fixed.
		if adminID == "" {
			log.Printf("auth: refusing admin request to %q: ADMIN_ID is not set (server misconfiguration)", c.Request.URL.Path)
			c.AbortWithStatusJSON(
				http.StatusInternalServerError,
				models.ErrorResponse{
					Status: http.StatusInternalServerError,
					Err:    "admin authorization is not configured",
				})

			return
		}

		// c.Get never panics; a missing uid simply fails the check below.
		uid, ok := c.Get("uid")
		if !ok || uid != adminID {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				models.ErrorResponse{
					Status: http.StatusUnauthorized,
					Err:    "admin privileges required",
				})

			return
		}

		c.Next()
	}
}
