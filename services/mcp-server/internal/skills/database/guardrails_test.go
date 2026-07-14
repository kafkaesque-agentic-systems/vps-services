package database

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ---------------------------------------------------------------------------
// Namespace allowlist
// ---------------------------------------------------------------------------

func TestResolveNamespace(t *testing.T) {
	t.Setenv(envMongoDatabase, "")

	ns, err := resolveNamespace("qdata")
	if err != nil {
		t.Fatalf("resolveNamespace(qdata): %v", err)
	}
	if ns.database != defaultAppDatabase || ns.collection != "qdata" {
		t.Errorf("qdata resolved to %s, want %s.qdata", ns, defaultAppDatabase)
	}

	ns, err = resolveNamespace("tdata")
	if err != nil {
		t.Fatalf("resolveNamespace(tdata): %v", err)
	}
	if ns.database != tarotDatabase {
		t.Errorf("tdata database = %q, want %q", ns.database, tarotDatabase)
	}
}

func TestResolveNamespaceHonorsMongoDatabaseEnv(t *testing.T) {
	t.Setenv(envMongoDatabase, "customdb")
	ns, err := resolveNamespace("users")
	if err != nil {
		t.Fatalf("resolveNamespace(users): %v", err)
	}
	if ns.database != "customdb" {
		t.Errorf("users database = %q, want customdb", ns.database)
	}
}

func TestResolveNamespaceRejectsUnknownDeterministically(t *testing.T) {
	_, err1 := resolveNamespace("admin")
	_, err2 := resolveNamespace("admin")
	if err1 == nil || err2 == nil {
		t.Fatal("resolveNamespace(admin): want error — admin must never be reachable")
	}
	if err1.Error() != err2.Error() {
		t.Errorf("error text is unstable:\n%q\n%q", err1, err2)
	}
	if want := "qdata, tdata, tokens, users"; !strings.Contains(err1.Error(), want) {
		t.Errorf("error = %q, want sorted allowlist %q", err1, want)
	}
}

// ---------------------------------------------------------------------------
// Bounding
// ---------------------------------------------------------------------------

func TestClampLimit(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, defaultFindLimit}, {-5, defaultFindLimit},
		{1, 1}, {20, 20}, {maxFindLimit, maxFindLimit},
		{51, maxFindLimit}, {5000, maxFindLimit},
	}
	for _, c := range cases {
		if got := clampLimit(c.in); got != c.want {
			t.Errorf("clampLimit(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampSkip(t *testing.T) {
	cases := []struct{ in, want int }{
		{-1, 0}, {0, 0}, {100, 100}, {maxSkip, maxSkip}, {maxSkip + 1, maxSkip},
	}
	for _, c := range cases {
		if got := clampSkip(c.in); got != c.want {
			t.Errorf("clampSkip(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestRenderDocumentsRespectsByteBudget(t *testing.T) {
	big := strings.Repeat("x", 20<<10) // 20 KiB per doc → budget fits 2
	docs := []bson.D{
		{{Key: "v", Value: big}},
		{{Key: "v", Value: big}},
		{{Key: "v", Value: big}},
	}
	res, err := renderDocuments(docs, false)
	if err != nil {
		t.Fatalf("renderDocuments: %v", err)
	}
	if !res.truncated {
		t.Error("truncated = false, want true for 60 KiB of documents against a 48 KiB budget")
	}
	if res.rendered >= len(docs) {
		t.Errorf("rendered = %d, want fewer than %d", res.rendered, len(docs))
	}
	if len(res.text) > maxResultBytes {
		t.Errorf("rendered output %d bytes exceeds the %d budget", len(res.text), maxResultBytes)
	}
}

// ---------------------------------------------------------------------------
// Extended JSON parsing
// ---------------------------------------------------------------------------

func TestParseDocument(t *testing.T) {
	doc, err := parseDocument("filter", `{"_id": {"$oid": "64ab0c1d2e3f405162738495"}}`)
	if err != nil {
		t.Fatalf("parseDocument with $oid: %v", err)
	}
	if len(doc) != 1 || doc[0].Key != "_id" {
		t.Errorf("parsed doc = %v, want single _id element", doc)
	}

	if _, err := parseDocument("filter", ""); err == nil {
		t.Error("empty string: want error")
	}
	if _, err := parseDocument("filter", "{not json"); err == nil {
		t.Error("malformed JSON: want error")
	} else if !strings.Contains(err.Error(), "'filter'") {
		t.Errorf("error %q must name the offending field", err)
	}
}

func TestParseDocumentArray(t *testing.T) {
	docs, err := parseDocumentArray("pipeline", `[{"$match": {}}, {"$sort": {"a": 1}}]`)
	if err != nil {
		t.Fatalf("parseDocumentArray: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("len = %d, want 2", len(docs))
	}
	if _, err := parseDocumentArray("pipeline", `{"$match": {}}`); err == nil {
		t.Error("object instead of array: want error")
	}
}

// ---------------------------------------------------------------------------
// Operator bans & write-filter guards
// ---------------------------------------------------------------------------

func TestFindForbiddenOperatorNested(t *testing.T) {
	doc, err := parseDocument("filter", `{"$or": [{"a": 1}, {"$where": "this.a > 1"}]}`)
	if err != nil {
		t.Fatalf("parseDocument: %v", err)
	}
	if got := findForbiddenOperator(doc, bannedEverywhere); got != "$where" {
		t.Errorf("findForbiddenOperator = %q, want $where (nested inside $or array)", got)
	}
}

func TestValidateWriteFilterRejectsEmpty(t *testing.T) {
	err := validateWriteFilter("db_delete", bson.D{})
	if err == nil {
		t.Fatal("empty filter: want rejection — this is the collection-wipe guard")
	}
	if !strings.Contains(err.Error(), "EVERY document") {
		t.Errorf("error %q should explain the blast radius", err)
	}
	if err := validateWriteFilter("db_delete", bson.D{{Key: "email", Value: "a@b.com"}}); err != nil {
		t.Errorf("selective filter rejected: %v", err)
	}
}

func TestValidateUpdateDocument(t *testing.T) {
	ok, err := parseDocument("update", `{"$set": {"granted": "true"}}`)
	if err != nil {
		t.Fatalf("parseDocument: %v", err)
	}
	if err := validateUpdateDocument(ok); err != nil {
		t.Errorf("operator update rejected: %v", err)
	}

	bare, err := parseDocument("update", `{"granted": "true"}`)
	if err != nil {
		t.Fatalf("parseDocument: %v", err)
	}
	if err := validateUpdateDocument(bare); err == nil {
		t.Error("bare replacement document: want rejection")
	}
	if err := validateUpdateDocument(bson.D{}); err == nil {
		t.Error("empty update: want rejection")
	}
}

func TestValidatePipelineGuards(t *testing.T) {
	t.Setenv(envMongoDatabase, "")
	target := namespace{database: defaultAppDatabase, collection: "qdata"}

	cases := []struct {
		name     string
		pipeline string
		wantErr  string
	}{
		{"out banned", `[{"$out": "stolen"}]`, "$out"},
		{"merge banned", `[{"$merge": {"into": "stolen"}}]`, "$merge"},
		{"nested where banned", `[{"$match": {"$where": "1"}}]`, "$where"},
		{"lookup unknown target", `[{"$lookup": {"from": "admincoll", "localField": "a", "foreignField": "b", "as": "j"}}]`, "unknown collection"},
		{"lookup cross-db", `[{"$lookup": {"from": "tdata", "localField": "a", "foreignField": "b", "as": "j"}}]`, "cross-database"},
		{"multi-key stage", `[{"$match": {}, "$sort": {"a": 1}}]`, "exactly one"},
		{"empty pipeline", `[]`, "at least one"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pipeline, err := parseDocumentArray("pipeline", c.pipeline)
			if err != nil {
				t.Fatalf("parseDocumentArray: %v", err)
			}
			err = validatePipeline(pipeline, target)
			if err == nil {
				t.Fatalf("validatePipeline(%s): want error", c.pipeline)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %q, want mention of %q", err, c.wantErr)
			}
		})
	}

	// Legitimate same-database lookup must pass.
	good, err := parseDocumentArray("pipeline",
		`[{"$lookup": {"from": "users", "localField": "uid", "foreignField": "uid", "as": "owner"}}]`)
	if err != nil {
		t.Fatalf("parseDocumentArray: %v", err)
	}
	if err := validatePipeline(good, target); err != nil {
		t.Errorf("same-db allowlisted $lookup rejected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Redaction
// ---------------------------------------------------------------------------

func TestRedactSecrets(t *testing.T) {
	doc := bson.D{
		{Key: "email", Value: "a@b.com"},
		{Key: "authorization", Value: "super-secret-token"},
		{Key: "nested", Value: bson.D{{Key: "authorization", Value: "another-secret"}}},
	}
	redacted, ok := redactSecrets(doc).(bson.D)
	if !ok {
		t.Fatal("redactSecrets did not return bson.D")
	}

	top, _ := redacted[1].Value.(string)
	if !strings.HasPrefix(top, "[REDACTED sha256:") {
		t.Errorf("top-level authorization = %q, want redaction marker", top)
	}
	nested, _ := redacted[2].Value.(bson.D)
	inner, _ := nested[0].Value.(string)
	if !strings.HasPrefix(inner, "[REDACTED sha256:") {
		t.Errorf("nested authorization = %q, want redaction marker", inner)
	}
	if email, _ := redacted[0].Value.(string); email != "a@b.com" {
		t.Errorf("email was altered: %q", email)
	}
	if strings.Contains(top, "super-secret-token") {
		t.Error("redaction leaked the raw secret")
	}
}

// ---------------------------------------------------------------------------
// Credentials & validation
// ---------------------------------------------------------------------------

func TestValidateEmail(t *testing.T) {
	valid := []string{"a@b.com", "dev+tag@example.co.uk", "x_y%z@my-host.org"}
	for _, e := range valid {
		if _, err := validateEmail(e); err != nil {
			t.Errorf("validateEmail(%q): unexpected error %v", e, err)
		}
	}
	invalid := []string{"", "not-an-email", "a b@c.com", "x'}); db.users.drop(); //@evil.com", "a@b"}
	for _, e := range invalid {
		if _, err := validateEmail(e); err == nil {
			t.Errorf("validateEmail(%q): want error", e)
		}
	}
}

func TestGenerateUserCredentials(t *testing.T) {
	uid1, tok1, err := generateUserCredentials()
	if err != nil {
		t.Fatalf("generateUserCredentials: %v", err)
	}
	uid2, tok2, err := generateUserCredentials()
	if err != nil {
		t.Fatalf("generateUserCredentials: %v", err)
	}
	if len(uid1) != uidRandomBytes*2 { // hex doubles length
		t.Errorf("uid length = %d, want %d", len(uid1), uidRandomBytes*2)
	}
	if uid1 == uid2 || tok1 == tok2 {
		t.Error("two credential generations produced identical output")
	}
}

// ---------------------------------------------------------------------------
// Config resolution
// ---------------------------------------------------------------------------

func TestResolveMongoConfigPrecedence(t *testing.T) {
	t.Setenv(envMongoURI, "")
	t.Setenv(envMongoUser, "scoped")
	t.Setenv(envMongoPass, "p@ss:word")
	t.Setenv(envRootUser, "root")
	t.Setenv(envRootPass, "rootpw")
	t.Setenv(envMongoHost, "")

	cfg, err := resolveMongoConfig()
	if err != nil {
		t.Fatalf("resolveMongoConfig: %v", err)
	}
	if cfg.usingRootCredentials {
		t.Error("scoped credentials present but root fallback used")
	}
	if !strings.Contains(cfg.uri, "scoped:") {
		t.Errorf("uri does not use the scoped user")
	}
	if strings.Contains(cfg.uri, "p@ss:word") {
		t.Error("password not URL-escaped in URI")
	}
	if cfg.host != defaultMongoHost {
		t.Errorf("host = %q, want %q", cfg.host, defaultMongoHost)
	}
}

func TestResolveMongoConfigRootFallbackAndFailure(t *testing.T) {
	t.Setenv(envMongoURI, "")
	t.Setenv(envMongoUser, "")
	t.Setenv(envMongoPass, "")
	t.Setenv(envRootUser, "root")
	t.Setenv(envRootPass, "rootpw")

	cfg, err := resolveMongoConfig()
	if err != nil {
		t.Fatalf("resolveMongoConfig: %v", err)
	}
	if !cfg.usingRootCredentials {
		t.Error("usingRootCredentials = false, want true for root fallback")
	}

	t.Setenv(envRootUser, "")
	t.Setenv(envRootPass, "")
	if _, err := resolveMongoConfig(); err == nil {
		t.Fatal("no credentials at all: want error")
	} else if !strings.Contains(err.Error(), envMongoUser) {
		t.Errorf("error %q should name the preferred variables", err)
	}
}

func TestWritesEnabledIsStrict(t *testing.T) {
	for _, v := range []string{"", "false", "1", "TRUE", "yes"} {
		t.Setenv(envAllowWrites, v)
		if writesEnabled() {
			t.Errorf("writesEnabled() = true for %q — must fail closed for anything but exactly \"true\"", v)
		}
	}
	t.Setenv(envAllowWrites, "true")
	if !writesEnabled() {
		t.Error("writesEnabled() = false for \"true\"")
	}
}

// ---------------------------------------------------------------------------
// Handler-level self-healing contract (no live MongoDB needed: these fail at
// validation, before any connection attempt)
// ---------------------------------------------------------------------------

func TestHandlersReturnIsErrorWithNilGoError(t *testing.T) {
	t.Setenv(envMongoURI, "")
	t.Setenv(envMongoUser, "")
	t.Setenv(envMongoPass, "")
	t.Setenv(envRootUser, "")
	t.Setenv(envRootPass, "")

	// Bad filter JSON — rejected in validation.
	res, _, err := handleDbFind(t.Context(), nil, FindInput{Collection: "qdata", Filter: "{bad"})
	if err != nil {
		t.Fatalf("handleDbFind returned a Go error: %v", err)
	}
	if !res.IsError {
		t.Error("handleDbFind(bad filter): IsError = false, want true")
	}

	// Empty delete filter — the wipe guard.
	res, _, err = handleDbDelete(t.Context(), nil, DeleteInput{Collection: "users", Filter: "{}"})
	if err != nil {
		t.Fatalf("handleDbDelete returned a Go error: %v", err)
	}
	if !res.IsError {
		t.Error("handleDbDelete(empty filter): IsError = false, want true")
	}

	// Unconfigured credentials — self-healing config error, not a panic.
	res, _, err = handleDbCollections(t.Context(), nil, CollectionsInput{})
	if err != nil {
		t.Fatalf("handleDbCollections returned a Go error: %v", err)
	}
	if !res.IsError {
		t.Error("handleDbCollections(no credentials): IsError = false, want true")
	}

	// Invalid email — user tooling gate.
	res, _, err = handleUserProvision(t.Context(), nil, UserProvisionInput{Email: "not-an-email"})
	if err != nil {
		t.Fatalf("handleUserProvision returned a Go error: %v", err)
	}
	if !res.IsError {
		t.Error("handleUserProvision(bad email): IsError = false, want true")
	}

	// Invalid ObjectId — lookup gate.
	res, _, err = handleQuoteOwnerLookup(t.Context(), nil, QuoteOwnerInput{QuoteID: "nope"})
	if err != nil {
		t.Fatalf("handleQuoteOwnerLookup returned a Go error: %v", err)
	}
	if !res.IsError {
		t.Error("handleQuoteOwnerLookup(bad id): IsError = false, want true")
	}
}
