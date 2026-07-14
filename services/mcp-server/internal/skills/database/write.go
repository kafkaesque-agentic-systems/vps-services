package database

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// InsertInput is the input schema for db_insert.
type InsertInput struct {
	Collection string `json:"collection" jsonschema:"Required. Target collection: qdata, tdata, tokens, or users."`
	Documents  string `json:"documents" jsonschema:"Required. Extended JSON ARRAY of documents as a string (max 25), e.g. [{\"quote\": \"...\", \"attribution\": \"...\", \"ueid\": \"...\", \"uid\": \"...\"}]. Inserts are ordered; a duplicate-key error reports the offending document."`
}

// UpdateInput is the input schema for db_update.
type UpdateInput struct {
	Collection string `json:"collection" jsonschema:"Required. Target collection: qdata, tdata, tokens, or users."`
	Filter     string `json:"filter" jsonschema:"Required. NON-EMPTY Extended JSON query object as a string; an empty {} filter is always rejected. Example: {\"email\": \"a@b.com\"}."`
	Update     string `json:"update" jsonschema:"Required. Extended JSON update document as a string using $ operators only, e.g. {\"$set\": {\"granted\": \"true\"}}. Bare replacement documents are rejected."`

	Many bool `json:"many,omitempty" jsonschema:"Optional, default false (updates ONE document). Setting true requires expected_matches and is capped at 100 affected documents."`

	ExpectedMatches int `json:"expected_matches,omitempty" jsonschema:"Required when many=true: the exact match count you obtained from db_count with the SAME filter. The server re-counts and aborts on mismatch."`

	Upsert bool `json:"upsert,omitempty" jsonschema:"Optional, default false. Insert a new document when nothing matches. Only valid with many=false."`
}

// DeleteInput is the input schema for db_delete.
type DeleteInput struct {
	Collection string `json:"collection" jsonschema:"Required. Target collection: qdata, tdata, tokens, or users."`
	Filter     string `json:"filter" jsonschema:"Required. NON-EMPTY Extended JSON query object as a string; an empty {} filter is always rejected — collection wipes are host-only operations."`

	Many bool `json:"many,omitempty" jsonschema:"Optional, default false (deletes ONE document). Setting true requires expected_matches and is capped at 100 affected documents."`

	ExpectedMatches int `json:"expected_matches,omitempty" jsonschema:"Required when many=true: the exact match count you obtained from db_count with the SAME filter. The server re-counts and aborts on mismatch."`
}

// auditWrite emits the guardrail-#7 audit line. Filters are logged as digests,
// never verbatim (they may embed emails or tokens).
func auditWrite(tool string, ns namespace, detail string) {
	log.Printf("skills/database: AUDIT %s on %s: %s", tool, ns, detail)
}

// guardManyWrite implements the two-phase handshake for many=true operations:
// re-count the filter server-side and abort on any mismatch with the caller's
// expectation, and enforce the absolute multi-write ceiling.
func guardManyWrite(ctx context.Context, tool string, coll *mongo.Collection, filter bson.D, expected int) error {
	if expected <= 0 {
		return fmt.Errorf(
			"%s with many=true requires expected_matches (> 0): first run db_count with the SAME filter, "+
				"then pass its result as expected_matches",
			tool,
		)
	}

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("pre-flight count for %s: %w", tool, err)
	}
	if count == 0 {
		return fmt.Errorf(
			"%s aborted: the filter currently matches 0 documents (expected_matches was %d); "+
				"the data changed or the filter is wrong — re-run db_count and retry",
			tool, expected,
		)
	}
	if count > maxManyWriteDocs {
		return fmt.Errorf(
			"%s aborted: the filter matches %d documents, above the hard multi-write ceiling of %d; "+
				"operations at that scale must be performed by an operator on the host, not via MCP",
			tool, count, maxManyWriteDocs,
		)
	}
	if count != int64(expected) {
		return fmt.Errorf(
			"%s aborted: expected_matches=%d but the filter currently matches %d documents; "+
				"re-run db_count with this exact filter and retry with the fresh count",
			tool, expected, count,
		)
	}
	return nil
}

// handleDbInsert implements db_insert (ordered, bounded batch insert).
func handleDbInsert(ctx context.Context, _ *mcp.CallToolRequest, in InsertInput) (*mcp.CallToolResult, any, error) {
	docs, err := parseDocumentArray("documents", in.Documents)
	if err != nil {
		return errorResult("db_insert: %v", err), nil, nil
	}
	if len(docs) == 0 {
		return errorResult("db_insert: 'documents' must contain at least one document"), nil, nil
	}
	if len(docs) > maxInsertDocs {
		return errorResult(
			"db_insert: %d documents exceeds the per-call cap of %d; split the batch into smaller calls",
			len(docs), maxInsertDocs,
		), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, in.Collection)
	if err != nil {
		return errorResult("db_insert: %v", err), nil, nil
	}

	anyDocs := make([]any, len(docs))
	for i, d := range docs {
		anyDocs[i] = d
	}

	result, err := coll.InsertMany(opCtx, anyDocs)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return errorResult(
				"db_insert on %s: duplicate key — a unique index rejected one of the documents "+
					"(qdata.ueid and tokens.email are unique). Use db_find to inspect the existing "+
					"document, then adjust or drop the conflicting insert: %v",
				ns, err,
			), nil, nil
		}
		return errorResult("db_insert on %s: %v", ns, err), nil, nil
	}

	auditWrite("db_insert", ns, fmt.Sprintf("inserted=%d", len(result.InsertedIDs)))
	return textResult(fmt.Sprintf(
		"status: ok\ntool: db_insert\nnamespace: %s\ninserted: %d\ninserted_ids: %v",
		ns, len(result.InsertedIDs), result.InsertedIDs,
	)), nil, nil
}

// handleDbUpdate implements db_update: single-document by default, operator-
// style updates only, empty filters rejected, multi-updates gated behind the
// count handshake and the 100-document ceiling.
func handleDbUpdate(ctx context.Context, _ *mcp.CallToolRequest, in UpdateInput) (*mcp.CallToolResult, any, error) {
	filter, err := parseDocument("filter", in.Filter)
	if err != nil {
		return errorResult("db_update: %v", err), nil, nil
	}
	if err := validateWriteFilter("db_update", filter); err != nil {
		return errorResult("db_update: %v", err), nil, nil
	}

	update, err := parseDocument("update", in.Update)
	if err != nil {
		return errorResult("db_update: %v", err), nil, nil
	}
	if err := validateUpdateDocument(update); err != nil {
		return errorResult("db_update: %v", err), nil, nil
	}

	if in.Many && in.Upsert {
		return errorResult(
			"db_update: upsert=true cannot be combined with many=true — an upsert that misses creates "+
				"exactly one document, which contradicts a multi-document intent; choose one",
		), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, in.Collection)
	if err != nil {
		return errorResult("db_update: %v", err), nil, nil
	}

	var result *mongo.UpdateResult
	if in.Many {
		if err := guardManyWrite(opCtx, "db_update", coll, filter, in.ExpectedMatches); err != nil {
			return errorResult("db_update: %v", err), nil, nil
		}
		result, err = coll.UpdateMany(opCtx, filter, update)
	} else {
		result, err = coll.UpdateOne(opCtx, filter, update, options.UpdateOne().SetUpsert(in.Upsert))
	}
	if err != nil {
		return errorResult("db_update on %s: %v", ns, err), nil, nil
	}

	auditWrite("db_update", ns, fmt.Sprintf(
		"many=%t filter_sha=%s matched=%d modified=%d upserted=%t",
		in.Many, filterDigest(filter), result.MatchedCount, result.ModifiedCount, result.UpsertedID != nil,
	))

	report := fmt.Sprintf(
		"status: ok\ntool: db_update\nnamespace: %s\nmany: %t\nmatched: %d\nmodified: %d\n",
		ns, in.Many, result.MatchedCount, result.ModifiedCount,
	)
	if result.UpsertedID != nil {
		report += fmt.Sprintf("upserted_id: %v\n", result.UpsertedID)
	}
	if result.MatchedCount == 0 && result.UpsertedID == nil {
		report += "note: nothing matched the filter; verify it with db_find, or pass upsert=true to create the document\n"
	}
	return textResult(report), nil, nil
}

// handleDbDelete implements db_delete with the same guardrail stack as
// db_update: non-empty filter, single-document default, count handshake and
// ceiling for many=true.
func handleDbDelete(ctx context.Context, _ *mcp.CallToolRequest, in DeleteInput) (*mcp.CallToolResult, any, error) {
	filter, err := parseDocument("filter", in.Filter)
	if err != nil {
		return errorResult("db_delete: %v", err), nil, nil
	}
	if err := validateWriteFilter("db_delete", filter); err != nil {
		return errorResult("db_delete: %v", err), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, in.Collection)
	if err != nil {
		return errorResult("db_delete: %v", err), nil, nil
	}

	var deleted int64
	if in.Many {
		if err := guardManyWrite(opCtx, "db_delete", coll, filter, in.ExpectedMatches); err != nil {
			return errorResult("db_delete: %v", err), nil, nil
		}
		result, delErr := coll.DeleteMany(opCtx, filter)
		if delErr != nil {
			return errorResult("db_delete on %s: %v", ns, delErr), nil, nil
		}
		deleted = result.DeletedCount
	} else {
		result, delErr := coll.DeleteOne(opCtx, filter)
		if delErr != nil {
			return errorResult("db_delete on %s: %v", ns, delErr), nil, nil
		}
		deleted = result.DeletedCount
	}

	auditWrite("db_delete", ns, fmt.Sprintf(
		"many=%t filter_sha=%s deleted=%d", in.Many, filterDigest(filter), deleted,
	))

	report := fmt.Sprintf(
		"status: ok\ntool: db_delete\nnamespace: %s\nmany: %t\ndeleted: %d\n",
		ns, in.Many, deleted,
	)
	if deleted == 0 {
		report += "note: nothing matched the filter — no documents were deleted; verify the filter with db_find\n"
	}
	return textResult(report), nil, nil
}
