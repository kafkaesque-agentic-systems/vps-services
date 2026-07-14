package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"kafka.local/quotes-api/handlers"
)

var ctx context.Context
var client *mongo.Client

var authoHandler *handlers.AuthoHandler
var quoteHandler *handlers.QuoteHandler
var tokenHandler *handlers.TokenHandler
var tarotHandler *handlers.TarotHandler

func init() {
	usr := os.Getenv("MONGO_INITDB_ROOT_USERNAME")
	pwd := os.Getenv("MONGO_INITDB_ROOT_PASSWORD")
	uri := fmt.Sprintf("mongodb://%s:%s@172.255.255.2:27017/test?authSource=admin", usr, pwd)

	ctx = context.Background()

	// STABILITY (Audit C-10): startup I/O gets an explicit deadline so a dead
	// database fails the boot in seconds instead of hanging indefinitely.
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// C-10 fix #1: the previous `client, err := mongo.Connect(...)` SHADOWED
	// the package-level client (leaving it nil forever) and NEVER CHECKED the
	// Connect error — it was immediately overwritten by Ping. Both are fixed:
	// assign to the package-level var and check each error independently.
	var err error
	client, err = mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("FAILED TO CONNECT TO DATABASE: %v", err) // fail closed at boot
	}

	if err = client.Ping(connectCtx, readpref.Primary()); err != nil {
		log.Fatalf("FAILED TO REACH DATABASE PRIMARY: %v", err)
	}

	log.Println("Connected to MongoDB")

	// Diagnostic only — a listing failure is logged, not fatal.
	if databases, err := client.ListDatabaseNames(connectCtx, bson.M{}); err != nil {
		log.Printf("warning: could not list database names: %v", err)
	} else {
		log.Printf("%v", databases)
	}

	quoteCollection := client.Database(os.Getenv("MONGO_DATABASE")).Collection("qdata")
	usersCollection := client.Database(os.Getenv("MONGO_DATABASE")).Collection("users")
	tokenCollection := client.Database(os.Getenv("MONGO_DATABASE")).Collection("tokens")
	tarotCollection := client.Database("tarotdb").Collection("tdata")

	quoteHandler = handlers.NewQuoteHandler(ctx, quoteCollection)
	authoHandler = handlers.NewAuthoHandler(ctx, usersCollection)
	tokenHandler = handlers.NewTokenHandler(ctx, tokenCollection)
	tarotHandler = handlers.NewTarotHandler(ctx, tarotCollection)
}

func main() {
	router := gin.Default()
	router.GET("/quote", quoteHandler.RandomQuoteHandler)
	router.GET("/quote/:id", quoteHandler.GetQuoteHandler)
	router.GET("/authors", quoteHandler.GetAuthorsHandler)
	router.GET("/authors/:name", quoteHandler.GetQuotesByAuthorHandler)
	router.POST("/quote/search", quoteHandler.SearchQuotesHandler)

	router.GET("/tarot/card", tarotHandler.RandomCardHandler)
	router.GET("/tarot/deck", tarotHandler.RandomDeckHandler)
	router.GET("/tarot/deck/:id", tarotHandler.TarotDeckHandler)
	router.GET("/tarot/decks", tarotHandler.ListDecksHandler)
	router.POST("/tarot/spread", tarotHandler.TarotSpreadHandler)

	authorized := router.Group("/")
	authorized.Use(authoHandler.AuthMiddleware())
	{
		authorized.POST("/quote", quoteHandler.AddQuoteHandler)
		authorized.PUT("/quote/:id", quoteHandler.UpdateQuoteHandler)
		authorized.DELETE("/quote/:id", quoteHandler.DeleteQuoteHandler)

		authorized.GET(
			"/admin/tokens",
			authoHandler.AdminAuthMiddleware(),
			tokenHandler.FetchTokenRequests,
		)

		authorized.GET(
			"/admin/tokens/:email",
			authoHandler.AdminAuthMiddleware(),
			tokenHandler.EmailExists,
		)

		authorized.POST(
			"/admin/tokens",
			authoHandler.AdminAuthMiddleware(),
			tokenHandler.AddTokenRequest,
		)

		authorized.PUT(
			"/admin/tokens/:id",
			authoHandler.AdminAuthMiddleware(),
			tokenHandler.UpdateTokenRequest,
		)

		// C-10: normalized the previously missing leading '/' (was
		// "admin/tokens/:id") so all route declarations are uniform.
		authorized.DELETE(
			"/admin/tokens/:id",
			authoHandler.AdminAuthMiddleware(),
			tokenHandler.DeleteTokenRequest,
		)
	}

	// STABILITY (Audit C-10): router.Run() previously produced an http.Server
	// with NO timeouts — a Slowloris client trickling header bytes could pin
	// connections indefinitely — and no shutdown path, so deploys severed
	// in-flight writes mid-transaction. The explicit server below sets
	// defensive deadlines and drains gracefully on SIGINT/SIGTERM.
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,  // Slowloris defense
		ReadTimeout:       15 * time.Second, // full request read deadline
		WriteTimeout:      30 * time.Second, // response write deadline
		IdleTimeout:       60 * time.Second, // keep-alive reaping
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Block until the orchestrator asks us to stop (docker stop → SIGTERM).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutdown signal received; draining connections")

	// Give in-flight requests up to 10s to complete, then release the
	// database client. Errors here are logged, not fatal — we are exiting
	// regardless, but we must not exit silently.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	if err := client.Disconnect(shutdownCtx); err != nil {
		log.Printf("mongo disconnect error: %v", err)
	}

	log.Println("shutdown complete")
}
