package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Bounding constants — the crash-prevention contract (doc.go guardrail #2).
const (
	// defaultFindLimit applies when the caller omits limit (or passes <= 0).
	defaultFindLimit = 20

	// maxFindLimit is the HARD row cap for every read tool. Enforced in Go and
	// pushed server-side; a larger request is clamped, never honored.
	maxFindLimit = 50

	// maxSkip caps pagination depth; deep skips are O(n) server work.
	maxSkip = 10_000

	// maxResultBytes is the response byte budget. Documents are rendered one at
	// a time and rendering stops (with a truncation notice) before the budget
	// is exceeded, so a handful of pathological multi-MB documents cannot blow
	// up the 256 MB container or the model's context window.
	maxResultBytes = 48 << 10 // 48 KiB

	// maxInsertDocs caps a single db_insert batch.
	maxInsertDocs = 25

	// maxManyWriteDocs is the absolute ceiling for many=true writes; anything
	// larger is host-scale work that does not belong in an LLM tool call.
	maxManyWriteDocs = 100

	// redactedFieldName is the field whose value is a live API secret in the
	// users collection. It is redacted in ALL output paths by default
	// (guardrail #6).
	redactedFieldName = "authorization"
)

// bannedEverywhere lists operators rejected in every filter, update document,
// and pipeline: all three execute caller-supplied JavaScript server-side,
// which is exactly the eval-injection class this skill exists to eliminate.
var bannedEverywhere = map[string]bool{
	"$where":       true,
	"$function":    true,
	"$accumulator": true,
}

// bannedPipelineStages lists aggregation stages that WRITE despite db_aggregate
// being a read tool.
var bannedPipelineStages = map[string]bool{
	"$out":   true,
	"$merge": true,
}

// parseDocument parses one Extended JSON object string into an order-preserving
// bson.D. Field names appear in every error so the LLM knows which argument to
// fix; examples show the expected shape including ObjectId syntax.
func parseDocument(field, src string) (bson.D, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, fmt.Errorf(
			"'%s' must be a MongoDB Extended JSON object passed as a string — it was empty; "+
				"examples: \"{}\" (match all), {\"email\": \"a@b.com\"}, {\"_id\": {\"$oid\": \"64ab0c...\"}}",
			field,
		)
	}
	var doc bson.D
	if err := bson.UnmarshalExtJSON([]byte(src), false, &doc); err != nil {
		return nil, fmt.Errorf(
			"'%s' is not valid MongoDB Extended JSON (%v); pass a JSON object string such as "+
				"{\"attribution\": \"aa milne\"} or {\"_id\": {\"$oid\": \"64ab0c...\"}}",
			field, err,
		)
	}
	return doc, nil
}

// parseDocumentArray parses an Extended JSON ARRAY string (pipelines, insert
// batches). bson.UnmarshalExtJSON requires a top-level document, so the array
// is wrapped in {"v": ...} before decoding — purely mechanical, no semantics.
func parseDocumentArray(field, src string) ([]bson.D, error) {
	src = strings.TrimSpace(src)
	if !strings.HasPrefix(src, "[") {
		return nil, fmt.Errorf(
			"'%s' must be a MongoDB Extended JSON ARRAY passed as a string, e.g. "+
				"[{\"$match\": {\"attribution\": \"rumi\"}}, {\"$sort\": {\"quote\": 1}}]",
			field,
		)
	}
	var wrapper struct {
		V []bson.D `bson:"v"`
	}
	if err := bson.UnmarshalExtJSON([]byte(`{"v":`+src+`}`), false, &wrapper); err != nil {
		return nil, fmt.Errorf("'%s' is not a valid MongoDB Extended JSON array (%v)", field, err)
	}
	return wrapper.V, nil
}

// findForbiddenOperator recursively walks a decoded BSON value (bson.D nests as
// bson.D / bson.A under the default registry) and returns the first banned key
// encountered, or "" when clean.
func findForbiddenOperator(value any, banned map[string]bool) string {
	switch v := value.(type) {
	case bson.D:
		for _, elem := range v {
			if banned[elem.Key] {
				return elem.Key
			}
			if found := findForbiddenOperator(elem.Value, banned); found != "" {
				return found
			}
		}
	case bson.A:
		for _, item := range v {
			if found := findForbiddenOperator(item, banned); found != "" {
				return found
			}
		}
	}
	return ""
}

// validateFilterOperators rejects filters carrying server-side-JS operators at
// any nesting depth (guardrail #5).
func validateFilterOperators(field string, doc bson.D) error {
	if op := findForbiddenOperator(doc, bannedEverywhere); op != "" {
		return fmt.Errorf(
			"'%s' contains the forbidden operator %q: server-side JavaScript is banned by this skill; "+
				"express the condition with standard query operators ($eq, $in, $regex, $gt, ...) instead",
			field, op,
		)
	}
	return nil
}

// validateUpdateDocument enforces operator-style updates: every top-level key
// must be a $-operator. Bare replacement documents are rejected because a
// mis-aimed replacement silently destroys all unnamed fields.
func validateUpdateDocument(doc bson.D) error {
	if len(doc) == 0 {
		return fmt.Errorf(
			"'update' must not be empty; use update operators, e.g. {\"$set\": {\"granted\": \"true\"}}",
		)
	}
	for _, elem := range doc {
		if !strings.HasPrefix(elem.Key, "$") {
			return fmt.Errorf(
				"'update' must use update operators — every top-level key must start with '$' "+
					"(e.g. {\"$set\": {...}}, {\"$unset\": {...}}, {\"$inc\": {...}}); found bare field %q. "+
					"Full-document replacement is not supported by this tool",
				elem.Key,
			)
		}
	}
	if op := findForbiddenOperator(doc, bannedEverywhere); op != "" {
		return fmt.Errorf("'update' contains the forbidden operator %q (server-side JavaScript is banned)", op)
	}
	return nil
}

// validateWriteFilter is the empty-filter guard for db_update / db_delete
// (guardrail #3). There is intentionally NO bypass flag.
func validateWriteFilter(tool string, doc bson.D) error {
	if len(doc) == 0 {
		return fmt.Errorf(
			"%s refuses an empty filter {}: it would affect EVERY document in the collection and "+
				"there is no override. Supply a selective filter; to change multiple documents, first run "+
				"db_count with the same filter, then call %s with many=true and expected_matches set to that count",
			tool, tool,
		)
	}
	return validateFilterOperators("filter", doc)
}

// validatePipeline enforces the aggregation guardrails: exactly one stage
// operator per stage document, no write stages, no server-side JS anywhere,
// and join/union targets restricted to allowlisted same-database collections.
func validatePipeline(pipeline []bson.D, target namespace) error {
	if len(pipeline) == 0 {
		return fmt.Errorf("'pipeline' must contain at least one stage, e.g. [{\"$match\": {}}]")
	}
	for i, stage := range pipeline {
		if len(stage) != 1 {
			return fmt.Errorf(
				"pipeline stage %d must contain exactly one stage operator, found %d keys; "+
					"each stage is its own object: [{\"$match\": {...}}, {\"$group\": {...}}]",
				i, len(stage),
			)
		}
		name := stage[0].Key
		if !strings.HasPrefix(name, "$") {
			return fmt.Errorf("pipeline stage %d key %q is not a stage operator (must start with '$')", i, name)
		}
		if bannedPipelineStages[name] {
			return fmt.Errorf(
				"pipeline stage %q is forbidden: db_aggregate is strictly read-only and %q writes to a "+
					"collection; use db_insert/db_update for mutations",
				name, name,
			)
		}
		switch name {
		case "$lookup", "$unionWith", "$graphLookup":
			if err := validateStageTarget(name, stage[0].Value, target); err != nil {
				return err
			}
		}
		if op := findForbiddenOperator(stage, bannedEverywhere); op != "" {
			return fmt.Errorf("pipeline contains the forbidden operator %q (server-side JavaScript is banned)", op)
		}
	}
	return nil
}

// validateStageTarget checks that a join/union stage references an allowlisted
// collection in the SAME database as the aggregation target ($lookup and
// friends are same-database operations; a foreign name would silently resolve
// to an unvetted collection).
func validateStageTarget(stage string, value any, target namespace) error {
	var from string
	switch v := value.(type) {
	case string: // $unionWith shorthand: {"$unionWith": "coll"}
		from = v
	case bson.D:
		for _, elem := range v {
			if elem.Key == "from" || elem.Key == "coll" {
				if s, ok := elem.Value.(string); ok {
					from = s
				}
			}
		}
	}
	if from == "" {
		return fmt.Errorf(
			"%s stage must name its source collection as a plain string in 'from' (or 'coll'); "+
				"sub-pipeline/$documents forms are not supported by this skill",
			stage,
		)
	}
	ns, err := resolveNamespace(from)
	if err != nil {
		return fmt.Errorf("%s stage: %v", stage, err)
	}
	if ns.database != target.database {
		return fmt.Errorf(
			"%s stage target %q lives in database %q but the aggregation runs in %q; "+
				"cross-database joins are not allowed",
			stage, from, ns.database, target.database,
		)
	}
	return nil
}

// redactSecrets deep-copies a decoded BSON value, replacing the value of any
// field named "authorization" (case-insensitive) with a sha256-prefix marker.
// The prefix lets an operator correlate a redacted value against a known token
// without the secret ever entering the model's context (guardrail #6).
func redactSecrets(value any) any {
	switch v := value.(type) {
	case bson.D:
		out := make(bson.D, 0, len(v))
		for _, elem := range v {
			if strings.EqualFold(elem.Key, redactedFieldName) {
				out = append(out, bson.E{Key: elem.Key, Value: redactionFor(elem.Value)})
				continue
			}
			out = append(out, bson.E{Key: elem.Key, Value: redactSecrets(elem.Value)})
		}
		return out
	case bson.A:
		out := make(bson.A, len(v))
		for i, item := range v {
			out[i] = redactSecrets(item)
		}
		return out
	default:
		return value
	}
}

// redactionFor renders the replacement marker for one secret value.
func redactionFor(value any) string {
	s, ok := value.(string)
	if !ok {
		return "[REDACTED]"
	}
	sum := sha256.Sum256([]byte(s))
	return "[REDACTED sha256:" + hex.EncodeToString(sum[:])[:12] + "]"
}

// renderResult carries the outcome of bounded document rendering.
type renderResult struct {
	// text holds newline-separated relaxed Extended JSON documents.
	text string

	// rendered counts documents actually included in text.
	rendered int

	// truncated is true when the byte budget stopped rendering early.
	truncated bool
}

// renderDocuments serializes documents one at a time against the response byte
// budget (guardrail #2, layer 2). redact controls authorization-field masking.
func renderDocuments(docs []bson.D, redact bool) (renderResult, error) {
	var b strings.Builder
	var res renderResult
	for _, doc := range docs {
		out := any(doc)
		if redact {
			out = redactSecrets(doc)
		}
		raw, err := bson.MarshalExtJSON(out, false, false)
		if err != nil {
			return res, fmt.Errorf("render document as Extended JSON: %w", err)
		}
		if b.Len()+len(raw)+1 > maxResultBytes {
			res.truncated = true
			break
		}
		b.Write(raw)
		b.WriteByte('\n')
		res.rendered++
	}
	res.text = b.String()
	return res, nil
}

// clampLimit applies the default and the hard row cap (guardrail #2, layer 1).
func clampLimit(requested int) int {
	if requested <= 0 {
		return defaultFindLimit
	}
	if requested > maxFindLimit {
		return maxFindLimit
	}
	return requested
}

// clampSkip bounds pagination depth.
func clampSkip(requested int) int {
	if requested <= 0 {
		return 0
	}
	if requested > maxSkip {
		return maxSkip
	}
	return requested
}

// filterDigest produces a short, secret-free fingerprint of a filter for audit
// log lines (guardrail #7): filters may embed emails or tokens, so the raw
// document must not be logged verbatim.
func filterDigest(doc bson.D) string {
	raw, err := bson.MarshalExtJSON(doc, true, false)
	if err != nil {
		return "unmarshalable"
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:12]
}
