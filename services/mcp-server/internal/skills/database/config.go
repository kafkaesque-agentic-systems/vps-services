package database

import (
	"cmp"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Environment variables consumed by this skill. Credential resolution follows a
// strict precedence documented on resolveMongoConfig.
const (
	// envMongoURI is the full-connection-string override. When set it wins
	// outright — useful for TLS/replica-set/local-dev topologies that the
	// component-wise construction below cannot express.
	envMongoURI = "MCP_MONGO_URI"

	// envMongoUser / envMongoPass are the PREFERRED credentials: the dedicated,
	// least-privilege mcp_agent MongoDB user (readWrite on qdb and tarotdb
	// only). Creation runbook: ARCHITECTURE.md "MCP database skill" section.
	envMongoUser = "MCP_MONGO_USERNAME"
	envMongoPass = "MCP_MONGO_PASSWORD"

	// envRootUser / envRootPass are the FALLBACK credentials, mirroring the api
	// service's connection logic. Using them works but triggers a WARNING log —
	// the MCP engine should not hold root when a scoped user suffices.
	envRootUser = "MONGO_INITDB_ROOT_USERNAME"
	envRootPass = "MONGO_INITDB_ROOT_PASSWORD"

	// envMongoHost overrides the MongoDB address. Defaults to the stack's
	// static IP, reachable from go-mcp (172.255.255.6) over the internal
	// `quotes` bridge network.
	envMongoHost = "MCP_MONGO_HOST"

	// envAllowWrites is the fail-closed write switch: db_insert, db_update,
	// db_delete, user_provision, and user_revoke are REGISTERED only when this
	// is exactly "true". Any other value (or unset) leaves the skill read-only.
	envAllowWrites = "MCP_DB_ALLOW_WRITES"

	// defaultMongoHost is the quotes-database container's static IP:port,
	// identical to the address hardcoded in api/src/main.go.
	defaultMongoHost = "172.255.255.2:27017"
)

// mongoConfig holds a resolved connection target. The uri field contains
// credentials and must NEVER be logged; use host for log lines and reports.
type mongoConfig struct {
	// uri is the complete connection string handed to the driver. SECRET.
	uri string

	// host is the credential-free address, safe for logs and error messages.
	host string

	// usingRootCredentials is true when the config fell back to the
	// MONGO_INITDB_ROOT_* pair; the client logs a warning in that case.
	usingRootCredentials bool
}

// writesEnabled reports whether destructive tools may be registered. The
// comparison is deliberately strict (exactly "true") so a typo fails CLOSED
// into read-only mode.
func writesEnabled() bool {
	return os.Getenv(envAllowWrites) == "true"
}

// resolveMongoConfig reads the environment and produces a connection target.
//
// Precedence (first match wins):
//
//  1. MCP_MONGO_URI — full connection string, used verbatim.
//  2. MCP_MONGO_USERNAME + MCP_MONGO_PASSWORD — scoped mcp_agent user
//     (preferred), combined with MCP_MONGO_HOST (default 172.255.255.2:27017)
//     and authSource=admin.
//  3. MONGO_INITDB_ROOT_USERNAME + MONGO_INITDB_ROOT_PASSWORD — root fallback,
//     same construction, flagged so the client can log a warning.
//
// Username and password are URL-escaped so credentials containing reserved
// characters (@ : / ? #) cannot corrupt the URI.
func resolveMongoConfig() (*mongoConfig, error) {
	if uri := strings.TrimSpace(os.Getenv(envMongoURI)); uri != "" {
		return &mongoConfig{uri: uri, host: "(address from " + envMongoURI + ")"}, nil
	}

	host := cmp.Or(strings.TrimSpace(os.Getenv(envMongoHost)), defaultMongoHost)

	user := strings.TrimSpace(os.Getenv(envMongoUser))
	pass := os.Getenv(envMongoPass)
	usingRoot := false
	if user == "" {
		user = strings.TrimSpace(os.Getenv(envRootUser))
		pass = os.Getenv(envRootPass)
		usingRoot = true
	}

	if user == "" || pass == "" {
		return nil, fmt.Errorf(
			"MongoDB credentials are not configured: set %s + %s (preferred scoped user), "+
				"or %s + %s (root fallback), or a full %s connection string in the go-mcp environment",
			envMongoUser, envMongoPass, envRootUser, envRootPass, envMongoURI,
		)
	}

	uri := fmt.Sprintf(
		"mongodb://%s:%s@%s/?authSource=admin",
		url.QueryEscape(user), url.QueryEscape(pass), host,
	)
	return &mongoConfig{uri: uri, host: host, usingRootCredentials: usingRoot}, nil
}
