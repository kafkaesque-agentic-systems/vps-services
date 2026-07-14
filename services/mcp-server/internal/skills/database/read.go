package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CollectionsInput is the input schema for db_collections (no parameters).
type CollectionsInput struct{}

// FindInput is the input schema for db_find.
type FindInput struct {
	// Collection is the exposed collection name, validated against the allowlist.
	Collection string `json:"collection" jsonschema:"Required. Target collection. Valid values: qdata (quotes), tdata (tarot decks), tokens (token requests), users (API users). Use db_collections to discover them."`

	// Filter is the query document as an Extended JSON string.
	Filter string `json:"filter" jsonschema:"Required. MongoDB Extended JSON query object as a string. Examples: \"{}\" (all documents, bounded), {\"attribution\": \"rumi\"}, {\"_id\": {\"$oid\": \"64ab0c...\"}}. $where is forbidden."`

	// Projection optionally selects fields, Extended JSON string.
	Projection string `json:"projection,omitempty" jsonschema:"Optional projection object as a string, e.g. {\"quote\": 1, \"attribution\": 1, \"_id\": 0}. Use projections to stay within the 48 KiB response budget."`

	// Sort optionally orders results, Extended JSON string.
	Sort string `json:"sort,omitempty" jsonschema:"Optional sort object as a string, e.g. {\"attribution\": 1} or {\"_id\": -1}."`

	// Limit caps returned documents; clamped to the hard cap of 50.
	Limit int `json:"limit,omitempty" jsonschema:"Optional. Max documents to return. Default 20, HARD CAP 50 (larger values are clamped in Go). Paginate with skip for more."`

	// Skip offsets into the result set for pagination; capped at 10000.
	Skip int `json:"skip,omitempty" jsonschema:"Optional pagination offset (max 10000). The response reports next_skip for the following page."`

	// IncludeSecrets opts out of authorization-field redaction.
	IncludeSecrets bool `json:"include_secrets,omitempty" jsonschema:"Optional, default false. When false any 'authorization' field is redacted to a sha256 marker. Set true ONLY when the raw API token is genuinely required."`
}

// CountInput is the input schema for db_count.
type CountInput struct {
	Collection string `json:"collection" jsonschema:"Required. Target collection: qdata, tdata, tokens, or users."`
	Filter     string `json:"filter" jsonschema:"Required. Extended JSON query object as a string; \"{}\" counts the whole collection. Run this before any many=true write to obtain expected_matches."`
}

// AggregateInput is the input schema for db_aggregate.
type AggregateInput struct {
	Collection string `json:"collection" jsonschema:"Required. Target collection: qdata, tdata, tokens, or users."`
	Pipeline   string `json:"pipeline" jsonschema:"Required. Extended JSON ARRAY of stages as a string, e.g. [{\"$match\": {\"attribution\": \"rumi\"}}, {\"$group\": {\"_id\": \"$attribution\", \"n\": {\"$sum\": 1}}}]. $out/$merge/$where/$function/$accumulator are forbidden; $lookup targets must be allowlisted same-database collections. A terminal $limit is always appended in Go."`
	Limit      int    `json:"limit,omitempty" jsonschema:"Optional. Max result documents. Default 20, HARD CAP 50 (enforced via an appended $limit stage)."`

	IncludeSecrets bool `json:"include_secrets,omitempty" jsonschema:"Optional, default false. When false any 'authorization' field in results is redacted."`
}

// handleDbCollections implements db_collections: the agent's discovery entry
// point. It reports each allowlisted namespace with an estimated document
// count, plus the skill's bounding limits so the agent can plan queries.
func handleDbCollections(ctx context.Context, _ *mcp.CallToolRequest, _ CollectionsInput) (*mcp.CallToolResult, any, error) {
	opCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	client, err := getClient(opCtx)
	if err != nil {
		return errorResult("db_collections: %v", err), nil, nil
	}

	var b strings.Builder
	b.WriteString("status: ok\ntool: db_collections\n\ncollections:\n")
	for _, name := range allowedCollectionNames() {
		ns := allowedNamespaces()[name]
		count, countErr := client.Database(ns.database).Collection(ns.collection).EstimatedDocumentCount(opCtx)
		if countErr != nil {
			b.WriteString(fmt.Sprintf("  - %s (%s): count_error: %v\n", name, ns, countErr))
			continue
		}
		b.WriteString(fmt.Sprintf("  - %s (%s): ~%d documents\n", name, ns, count))
	}
	b.WriteString(fmt.Sprintf(
		"\nlimits:\n  find/aggregate: default %d docs, hard cap %d, response budget %d bytes, max skip %d\n"+
			"  insert: max %d docs per call\n  many-writes: require expected_matches (via db_count), ceiling %d docs\n"+
			"  redaction: 'authorization' fields are masked unless include_secrets=true\n",
		defaultFindLimit, maxFindLimit, maxResultBytes, maxSkip, maxInsertDocs, maxManyWriteDocs,
	))
	return textResult(b.String()), nil, nil
}

// handleDbFind implements db_find with the full bounding contract: hard row
// cap (fetching cap+1 to detect more pages), byte budget, skip ceiling, and
// context-deadline time box.
func handleDbFind(ctx context.Context, _ *mcp.CallToolRequest, in FindInput) (*mcp.CallToolResult, any, error) {
	filter, err := parseDocument("filter", in.Filter)
	if err != nil {
		return errorResult("db_find: %v", err), nil, nil
	}
	if err := validateFilterOperators("filter", filter); err != nil {
		return errorResult("db_find: %v", err), nil, nil
	}

	limit := clampLimit(in.Limit)
	skip := clampSkip(in.Skip)

	// Fetch limit+1 so has_more is known without a second count query.
	opts := options.Find().SetLimit(int64(limit) + 1).SetSkip(int64(skip))
	if strings.TrimSpace(in.Projection) != "" {
		proj, projErr := parseDocument("projection", in.Projection)
		if projErr != nil {
			return errorResult("db_find: %v", projErr), nil, nil
		}
		opts.SetProjection(proj)
	}
	if strings.TrimSpace(in.Sort) != "" {
		sort, sortErr := parseDocument("sort", in.Sort)
		if sortErr != nil {
			return errorResult("db_find: %v", sortErr), nil, nil
		}
		opts.SetSort(sort)
	}

	opCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, in.Collection)
	if err != nil {
		return errorResult("db_find: %v", err), nil, nil
	}

	cursor, err := coll.Find(opCtx, filter, opts)
	if err != nil {
		return errorResult("db_find on %s: %v", ns, err), nil, nil
	}
	defer func() { _ = cursor.Close(opCtx) }()

	var docs []bson.D
	for cursor.Next(opCtx) {
		var doc bson.D
		if decErr := cursor.Decode(&doc); decErr != nil {
			return errorResult("db_find on %s: decode document: %v", ns, decErr), nil, nil
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return errorResult("db_find on %s: cursor error: %v", ns, err), nil, nil
	}

	hasMore := false
	if len(docs) > limit {
		hasMore = true
		docs = docs[:limit]
	}

	rendered, err := renderDocuments(docs, !in.IncludeSecrets)
	if err != nil {
		return errorResult("db_find on %s: %v", ns, err), nil, nil
	}
	if rendered.truncated {
		hasMore = true
	}

	report := fmt.Sprintf(
		"status: ok\ntool: db_find\nnamespace: %s\nreturned: %d\nlimit: %d (hard cap %d)\nskip: %d\n"+
			"has_more: %t\nnext_skip: %d\nsecrets_redacted: %t\n",
		ns, rendered.rendered, limit, maxFindLimit, skip, hasMore, skip+rendered.rendered, !in.IncludeSecrets,
	)
	if rendered.truncated {
		report += fmt.Sprintf(
			"note: output truncated at the %d-byte response budget after %d documents; "+
				"narrow the filter or add a projection to see more per page\n",
			maxResultBytes, rendered.rendered,
		)
	}
	if rendered.rendered == 0 {
		report += "note: no documents matched; verify field names and values with db_collections / a broader filter\n"
	}
	return textResult(report + "\n" + rendered.text), nil, nil
}

// handleDbCount implements db_count.
func handleDbCount(ctx context.Context, _ *mcp.CallToolRequest, in CountInput) (*mcp.CallToolResult, any, error) {
	filter, err := parseDocument("filter", in.Filter)
	if err != nil {
		return errorResult("db_count: %v", err), nil, nil
	}
	if err := validateFilterOperators("filter", filter); err != nil {
		return errorResult("db_count: %v", err), nil, nil
	}

	opCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, in.Collection)
	if err != nil {
		return errorResult("db_count: %v", err), nil, nil
	}

	count, err := coll.CountDocuments(opCtx, filter)
	if err != nil {
		return errorResult("db_count on %s: %v", ns, err), nil, nil
	}

	return textResult(fmt.Sprintf(
		"status: ok\ntool: db_count\nnamespace: %s\ncount: %d\n"+
			"hint: use this value as expected_matches for a many=true db_update/db_delete on the same filter",
		ns, count,
	)), nil, nil
}

// handleDbAggregate implements db_aggregate. A terminal $limit of cap+1 is
// ALWAYS appended (even after a caller-supplied $limit — appending is harmless
// and unconditional appending is simpler to reason about than detection), so
// an $unwind-style fan-out late in the pipeline still cannot exceed the cap.
func handleDbAggregate(ctx context.Context, _ *mcp.CallToolRequest, in AggregateInput) (*mcp.CallToolResult, any, error) {
	pipeline, err := parseDocumentArray("pipeline", in.Pipeline)
	if err != nil {
		return errorResult("db_aggregate: %v", err), nil, nil
	}

	ns, err := resolveNamespace(in.Collection)
	if err != nil {
		return errorResult("db_aggregate: %v", err), nil, nil
	}
	if err := validatePipeline(pipeline, ns); err != nil {
		return errorResult("db_aggregate: %v", err), nil, nil
	}

	limit := clampLimit(in.Limit)
	bounded := append(pipeline, bson.D{{Key: "$limit", Value: int64(limit) + 1}})

	opCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	coll, ns, err := collectionFor(opCtx, in.Collection)
	if err != nil {
		return errorResult("db_aggregate: %v", err), nil, nil
	}

	cursor, err := coll.Aggregate(opCtx, bounded)
	if err != nil {
		return errorResult("db_aggregate on %s: %v", ns, err), nil, nil
	}
	defer func() { _ = cursor.Close(opCtx) }()

	var docs []bson.D
	for cursor.Next(opCtx) {
		var doc bson.D
		if decErr := cursor.Decode(&doc); decErr != nil {
			return errorResult("db_aggregate on %s: decode document: %v", ns, decErr), nil, nil
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return errorResult("db_aggregate on %s: cursor error: %v", ns, err), nil, nil
	}

	hasMore := false
	if len(docs) > limit {
		hasMore = true
		docs = docs[:limit]
	}

	rendered, err := renderDocuments(docs, !in.IncludeSecrets)
	if err != nil {
		return errorResult("db_aggregate on %s: %v", ns, err), nil, nil
	}
	if rendered.truncated {
		hasMore = true
	}

	report := fmt.Sprintf(
		"status: ok\ntool: db_aggregate\nnamespace: %s\nreturned: %d\nlimit: %d (hard cap %d, enforced via appended $limit)\n"+
			"has_more: %t\nsecrets_redacted: %t\n",
		ns, rendered.rendered, limit, maxFindLimit, hasMore, !in.IncludeSecrets,
	)
	if rendered.truncated {
		report += fmt.Sprintf(
			"note: output truncated at the %d-byte response budget after %d documents; "+
				"add a $project stage or tighten $match to see more per page\n",
			maxResultBytes, rendered.rendered,
		)
	}
	return textResult(report + "\n" + rendered.text), nil, nil
}
