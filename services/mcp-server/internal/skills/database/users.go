package database

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// emailPattern mirrors the character class used by the hardened legacy scripts
// (Audit C-8). With the native driver there is no eval surface to protect, but
// the gate still rejects garbage before it becomes a stored document.
var emailPattern = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)

// hexObjectIDPattern matches a canonical 24-hex-character MongoDB ObjectId.
var hexObjectIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)

// Credential sizes, matching the legacy scripts exactly so provisioned users
// are indistinguishable from historical ones:
// uid = 16 random bytes hex-encoded (openssl rand -hex 16),
// token = 32 random bytes base64-encoded (openssl rand -base64 32).
const (
	uidRandomBytes   = 16
	tokenRandomBytes = 32
)

// UserProvisionInput is the input schema for user_provision.
type UserProvisionInput struct {
	Email string `json:"email" jsonschema:"Required. Email address for the new API user, e.g. dev@example.com. The generated uid and authorization token are returned ONCE in the response."`
}

// UserRevokeInput is the input schema for user_revoke.
type UserRevokeInput struct {
	Email string `json:"email" jsonschema:"Required. Email address of the API user to remove. Deletes at most one document."`
}

// UserListInput is the input schema for user_list.
type UserListInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Optional. Max users to return. Default 20, hard cap 50."`

	IncludeTokens bool `json:"include_tokens,omitempty" jsonschema:"Optional, default false. When false the authorization tokens are redacted to sha256 markers. Set true ONLY when the raw token is genuinely required."`
}

// QuoteOwnerInput is the input schema for quote_owner_lookup.
type QuoteOwnerInput struct {
	QuoteID string `json:"quote_id" jsonschema:"Required. The quote document's ObjectId as a 24-character hex string, e.g. 64ab0c1d2e3f405162738495."`
}

// generateUserCredentials produces the uid + authorization token for a new API
// user from crypto/rand (never math/rand — these are security credentials).
func generateUserCredentials() (uid, token string, err error) {
	uidBytes := make([]byte, uidRandomBytes)
	if _, err := rand.Read(uidBytes); err != nil {
		return "", "", fmt.Errorf("generate uid: %w", err)
	}
	tokenBytes := make([]byte, tokenRandomBytes)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate authorization token: %w", err)
	}
	return hex.EncodeToString(uidBytes), base64.StdEncoding.EncodeToString(tokenBytes), nil
}

// validateEmail centralizes the email gate with a self-healing message.
func validateEmail(email string) (string, error) {
	email = strings.TrimSpace(email)
	if !emailPattern.MatchString(email) {
		return "", fmt.Errorf(
			"%q is not a valid email address; expected a plain address such as dev@example.com "+
				"(no spaces, quotes, or display names)",
			email,
		)
	}
	return email, nil
}

// handleUserProvision implements user_provision (replaces scripts/add_user.sh).
//
// Improvement over the legacy script: the duplicate pre-check is best-effort
// UX; true atomicity comes from the recommended unique index on users.email
// (runbook in ARCHITECTURE.md) — a concurrent duplicate insert surfaces as a
// duplicate-key error rather than silently creating a second user.
func handleUserProvision(ctx context.Context, _ *mcp.CallToolRequest, in UserProvisionInput) (*mcp.CallToolResult, any, error) {
	email, err := validateEmail(in.Email)
	if err != nil {
		return errorResult("user_provision: %v", err), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, "users")
	if err != nil {
		return errorResult("user_provision: %v", err), nil, nil
	}

	existing, err := coll.CountDocuments(opCtx, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return errorResult("user_provision: duplicate pre-check on %s: %v", ns, err), nil, nil
	}
	if existing > 0 {
		return errorResult(
			"user_provision: a user with email %q already exists; use user_list to inspect it, "+
				"or user_revoke first if you intend to rotate their credentials",
			email,
		), nil, nil
	}

	uid, token, err := generateUserCredentials()
	if err != nil {
		return errorResult("user_provision: %v", err), nil, nil
	}

	if _, err := coll.InsertOne(opCtx, bson.D{
		{Key: "uid", Value: uid},
		{Key: "email", Value: email},
		{Key: "authorization", Value: token},
	}); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return errorResult(
				"user_provision: concurrent duplicate for %q rejected by the unique email index; "+
					"the user already exists — use user_list to inspect it",
				email,
			), nil, nil
		}
		return errorResult("user_provision: insert into %s: %v", ns, err), nil, nil
	}

	// AUDIT: the token itself is deliberately absent from the log line.
	auditWrite("user_provision", ns, fmt.Sprintf("email=%s uid=%s", email, uid))

	return textResult(fmt.Sprintf(
		"status: ok\ntool: user_provision\nemail: %s\nuid: %s\nauthorization_token: %s\n\n"+
			"IMPORTANT: this token is shown ONCE and is stored in plaintext in %s. "+
			"Deliver it to the user over a secure channel now; subsequent user_list calls redact it.",
		email, uid, token, ns,
	)), nil, nil
}

// handleUserRevoke implements user_revoke (replaces scripts/remove_user.sh).
func handleUserRevoke(ctx context.Context, _ *mcp.CallToolRequest, in UserRevokeInput) (*mcp.CallToolResult, any, error) {
	email, err := validateEmail(in.Email)
	if err != nil {
		return errorResult("user_revoke: %v", err), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, "users")
	if err != nil {
		return errorResult("user_revoke: %v", err), nil, nil
	}

	result, err := coll.DeleteOne(opCtx, bson.D{{Key: "email", Value: email}})
	if err != nil {
		return errorResult("user_revoke: delete from %s: %v", ns, err), nil, nil
	}
	if result.DeletedCount == 0 {
		return errorResult(
			"user_revoke: no user with email %q exists; run user_list to see current users "+
				"(the match is exact and case-sensitive)",
			email,
		), nil, nil
	}

	auditWrite("user_revoke", ns, "email="+email)
	return textResult(fmt.Sprintf(
		"status: ok\ntool: user_revoke\nemail: %s\ndeleted: 1\n"+
			"note: the user's API token is now invalid; existing sessions are unaffected because the api "+
			"service validates the Authorization header per request",
		email,
	)), nil, nil
}

// handleUserList implements user_list (replaces scripts/list_users.sh) — with
// the two fixes the script needed: a hard result bound, and token redaction by
// default so live credentials never enter the model context unrequested.
func handleUserList(ctx context.Context, _ *mcp.CallToolRequest, in UserListInput) (*mcp.CallToolResult, any, error) {
	limit := clampLimit(in.Limit)

	opCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, "users")
	if err != nil {
		return errorResult("user_list: %v", err), nil, nil
	}

	opts := options.Find().
		SetLimit(int64(limit)).
		SetSort(bson.D{{Key: "email", Value: 1}}). // deterministic ordering
		SetProjection(bson.D{
			{Key: "_id", Value: 0},
			{Key: "email", Value: 1},
			{Key: "uid", Value: 1},
			{Key: "authorization", Value: 1},
		})

	cursor, err := coll.Find(opCtx, bson.D{}, opts)
	if err != nil {
		return errorResult("user_list on %s: %v", ns, err), nil, nil
	}
	defer func() { _ = cursor.Close(opCtx) }()

	var docs []bson.D
	for cursor.Next(opCtx) {
		var doc bson.D
		if decErr := cursor.Decode(&doc); decErr != nil {
			return errorResult("user_list on %s: decode document: %v", ns, decErr), nil, nil
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return errorResult("user_list on %s: cursor error: %v", ns, err), nil, nil
	}

	rendered, err := renderDocuments(docs, !in.IncludeTokens)
	if err != nil {
		return errorResult("user_list on %s: %v", ns, err), nil, nil
	}

	return textResult(fmt.Sprintf(
		"status: ok\ntool: user_list\nnamespace: %s\nreturned: %d\nlimit: %d (hard cap %d)\ntokens_redacted: %t\n\n%s",
		ns, rendered.rendered, limit, maxFindLimit, !in.IncludeTokens, rendered.text,
	)), nil, nil
}

// handleQuoteOwnerLookup implements quote_owner_lookup (replaces
// scripts/find_user_by_post_id.sh): quote ObjectId → owner uid.
func handleQuoteOwnerLookup(ctx context.Context, _ *mcp.CallToolRequest, in QuoteOwnerInput) (*mcp.CallToolResult, any, error) {
	id := strings.TrimSpace(in.QuoteID)
	if !hexObjectIDPattern.MatchString(id) {
		return errorResult(
			"quote_owner_lookup: %q is not a valid ObjectId — expected exactly 24 hexadecimal characters, "+
				"e.g. 64ab0c1d2e3f405162738495 (find quote IDs via db_find on qdata)",
			id,
		), nil, nil
	}
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return errorResult("quote_owner_lookup: parse ObjectId %q: %v", id, err), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, "qdata")
	if err != nil {
		return errorResult("quote_owner_lookup: %v", err), nil, nil
	}

	var doc struct {
		UID         string `bson:"uid"`
		Attribution string `bson:"attribution"`
	}
	findErr := coll.FindOne(opCtx, bson.D{{Key: "_id", Value: oid}},
		options.FindOne().SetProjection(bson.D{
			{Key: "uid", Value: 1},
			{Key: "attribution", Value: 1},
		}),
	).Decode(&doc)
	if findErr != nil {
		if errors.Is(findErr, mongo.ErrNoDocuments) {
			return errorResult(
				"quote_owner_lookup: no quote with _id %s exists in %s; verify the id with db_find "+
					"(e.g. filter {\"attribution\": \"...\"} plus projection {\"_id\": 1})",
				id, ns,
			), nil, nil
		}
		return errorResult("quote_owner_lookup on %s: %v", ns, findErr), nil, nil
	}

	return textResult(fmt.Sprintf(
		"status: ok\ntool: quote_owner_lookup\nquote_id: %s\nowner_uid: %s\nattribution: %s\n"+
			"hint: match owner_uid against user_list's uid column to identify the user",
		id, doc.UID, doc.Attribution,
	)), nil, nil
}
