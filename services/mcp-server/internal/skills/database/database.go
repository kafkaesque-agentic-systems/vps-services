package database

import (
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool name constants — the public contract with clients. Namespacing: generic
// document tools use "db_", the user-management workflow tools use "user_" /
// "quote_" (they are domain workflows, not raw collection access).
const (
	toolNameCollections = "db_collections"
	toolNameFind        = "db_find"
	toolNameCount       = "db_count"
	toolNameAggregate   = "db_aggregate"

	toolNameInsert = "db_insert"
	toolNameUpdate = "db_update"
	toolNameDelete = "db_delete"

	toolNameUserProvision = "user_provision"
	toolNameUserRevoke    = "user_revoke"
	toolNameUserList      = "user_list"
	toolNameQuoteOwner    = "quote_owner_lookup"
)

// Register attaches the database skill's tools to the provided MCP server.
//
// # Read-only by default (approved decision #3)
//
// The write tools (db_insert, db_update, db_delete, user_provision,
// user_revoke) are registered ONLY when MCP_DB_ALLOW_WRITES=true. In read-only
// mode they are not merely disabled — they are never advertised, so the model
// cannot even attempt a mutation. This mirrors the fail-closed philosophy of
// the auth middleware: enabling destruction requires a deliberate operator
// action, never the absence of one.
func Register(server *mcp.Server) {
	valid := strings.Join(allowedCollectionNames(), ", ")

	// --- Read tools (always available) ---------------------------------------

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameCollections,
		Description: "Lists the MongoDB collections this server may access (" + valid + ") with estimated " +
			"document counts, plus the skill's bounding limits. Call this first to discover the data model. " +
			"Takes no arguments.",
	}, handleDbCollections)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameFind,
		Description: "Runs a bounded MongoDB find on an allowlisted collection (" + valid + "). " +
			"Pass 'filter' (and optionally projection/sort) as MongoDB Extended JSON strings — " +
			"ObjectIds as {\"$oid\": \"...\"}. Results are HARD-CAPPED at " + fmt.Sprint(maxFindLimit) +
			" documents (default " + fmt.Sprint(defaultFindLimit) + ") and a " + fmt.Sprint(maxResultBytes) +
			"-byte budget; paginate with skip/next_skip. 'authorization' fields are redacted unless " +
			"include_secrets=true.",
	}, handleDbFind)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameCount,
		Description: "Counts documents matching an Extended JSON filter on an allowlisted collection (" + valid + "). " +
			"Also the REQUIRED first step of the many=true write handshake: run db_count, then pass the result " +
			"as expected_matches to db_update/db_delete.",
	}, handleDbCount)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameAggregate,
		Description: "Runs a bounded, read-only aggregation pipeline on an allowlisted collection (" + valid + "). " +
			"Pass 'pipeline' as an Extended JSON array string. Forbidden: $out, $merge, $where, $function, " +
			"$accumulator; $lookup/$unionWith targets must be allowlisted same-database collections. " +
			"A terminal $limit (hard cap " + fmt.Sprint(maxFindLimit) + ") is always appended in Go.",
	}, handleDbAggregate)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameUserList,
		Description: "Lists API users (email, uid, authorization) from the users collection, sorted by email, " +
			"capped at " + fmt.Sprint(maxFindLimit) + ". Authorization tokens are REDACTED to sha256 markers " +
			"unless include_tokens=true. Replaces the legacy list_users.sh script.",
	}, handleUserList)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameQuoteOwner,
		Description: "Looks up which API user owns a quote: give the quote's ObjectId (24 hex chars) and receive " +
			"the owner's uid plus the quote's attribution. Replaces the legacy find_user_by_post_id.sh script.",
	}, handleQuoteOwnerLookup)

	if !writesEnabled() {
		log.Printf(
			"skills/database: registered READ tools %q, %q, %q, %q, %q, %q (read-only mode; set %s=true to enable write tools)",
			toolNameCollections, toolNameFind, toolNameCount, toolNameAggregate,
			toolNameUserList, toolNameQuoteOwner, envAllowWrites,
		)
		return
	}

	// --- Write tools (MCP_DB_ALLOW_WRITES=true only) --------------------------

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameInsert,
		Description: "Inserts up to " + fmt.Sprint(maxInsertDocs) + " documents (Extended JSON array string) into " +
			"an allowlisted collection (" + valid + "). Ordered; duplicate-key violations are reported with the " +
			"offending document so you can correct and retry.",
	}, handleDbInsert)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameUpdate,
		Description: "Updates documents in an allowlisted collection (" + valid + "). Requires a NON-EMPTY Extended " +
			"JSON filter and an operator-style update ({\"$set\": ...}); bare replacements are rejected. Default " +
			"updates ONE document; many=true requires expected_matches from a prior db_count and is capped at " +
			fmt.Sprint(maxManyWriteDocs) + " documents. upsert=true is valid only with many=false.",
	}, handleDbUpdate)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameDelete,
		Description: "Deletes documents from an allowlisted collection (" + valid + "). Requires a NON-EMPTY Extended " +
			"JSON filter — an empty {} filter is ALWAYS rejected (collection wipes are host-only operations). " +
			"Default deletes ONE document; many=true requires expected_matches from a prior db_count and is capped " +
			"at " + fmt.Sprint(maxManyWriteDocs) + " documents.",
	}, handleDbDelete)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameUserProvision,
		Description: "Provisions a new API user: validates the email, generates a crypto-random uid and " +
			"authorization token (same formats as the legacy add_user.sh), and inserts atomically into the users " +
			"collection. The token is returned ONCE — deliver it securely immediately. Fails if the email exists.",
	}, handleUserProvision)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameUserRevoke,
		Description: "Revokes an API user by exact email address (deletes at most one users document, invalidating " +
			"their token). Replaces the legacy remove_user.sh script. Fails self-healingly if no such user exists.",
	}, handleUserRevoke)

	log.Printf(
		"skills/database: registered READ tools %q, %q, %q, %q, %q, %q and WRITE tools %q, %q, %q, %q, %q (%s=true)",
		toolNameCollections, toolNameFind, toolNameCount, toolNameAggregate,
		toolNameUserList, toolNameQuoteOwner,
		toolNameInsert, toolNameUpdate, toolNameDelete,
		toolNameUserProvision, toolNameUserRevoke, envAllowWrites,
	)
}

// textResult builds a successful tool result carrying a single block of text.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// errorResult builds a tool result flagged as an error (IsError: true) whose
// text is a descriptive, self-healing message intended for the calling LLM —
// returned alongside a nil Go error so the guidance reaches the model as tool
// output rather than an opaque protocol failure (core directive 2.4).
func errorResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
	}
}
