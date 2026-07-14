package database

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

const (
	// envMongoDatabase names the application database, mirroring the api
	// service's use of the same variable. Defaults to "qdb" when unset.
	envMongoDatabase = "MONGO_DATABASE"

	// defaultAppDatabase is the production application database name.
	defaultAppDatabase = "qdb"

	// tarotDatabase is the fixed second database (hardcoded in the api service
	// the same way: client.Database("tarotdb")).
	tarotDatabase = "tarotdb"
)

// namespace binds an exposed collection name to its physical MongoDB location.
//
// Tools accept the SHORT collection name (e.g. "qdata"); the database half is
// resolved here and is never caller-controllable. This is the choke point that
// makes admin/config/local and arbitrary databases unreachable regardless of
// the privileges of the connected MongoDB user (defense in depth alongside the
// scoped mcp_agent account).
type namespace struct {
	database   string
	collection string
}

// String renders the fully qualified "db.collection" form for reports and logs.
func (ns namespace) String() string {
	return ns.database + "." + ns.collection
}

// appDatabaseName resolves the application database, defaulting to qdb. It is
// read per call (not cached) so tests can vary MONGO_DATABASE via t.Setenv and
// so a container restart with a changed env behaves predictably.
func appDatabaseName() string {
	return cmp.Or(strings.TrimSpace(os.Getenv(envMongoDatabase)), defaultAppDatabase)
}

// allowedNamespaces returns THE allowlist: the only four namespaces any tool in
// this package can touch. Adding a namespace here is a deliberate, reviewable
// security decision — never derive this map from user input or live server
// introspection.
func allowedNamespaces() map[string]namespace {
	appDB := appDatabaseName()
	return map[string]namespace{
		"qdata":  {database: appDB, collection: "qdata"},  // quotes
		"users":  {database: appDB, collection: "users"},  // API users + auth tokens
		"tokens": {database: appDB, collection: "tokens"}, // pending token requests
		"tdata":  {database: tarotDatabase, collection: "tdata"}, // tarot decks
	}
}

// allowedCollectionNames returns the exposed collection names SORTED, so every
// schema description and self-healing error lists them deterministically.
func allowedCollectionNames() []string {
	return slices.Sorted(maps.Keys(allowedNamespaces()))
}

// resolveNamespace validates an exposed collection name against the allowlist.
//
// The error text enumerates the valid names (sorted) so the calling LLM can
// immediately correct its next invocation.
func resolveNamespace(name string) (namespace, error) {
	name = strings.TrimSpace(name)
	ns, ok := allowedNamespaces()[name]
	if !ok {
		return namespace{}, fmt.Errorf(
			"unknown collection %q; valid collections: %s",
			name, strings.Join(allowedCollectionNames(), ", "),
		)
	}
	return ns, nil
}
