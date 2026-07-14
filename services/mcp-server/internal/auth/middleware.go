package auth

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// The expected shared secret is read from the environment exactly ONCE and
// cached for the lifetime of the process (Audit O-2). Previously the middleware
// called os.Getenv on every request; while cheap, it is needless work on the hot
// path. Since MCP_SECRET_TOKEN is fixed for a running container, a one-time read
// guarded by sync.Once is both faster and semantically clearer.
//
// Trade-off (accepted): because the value is cached, changing MCP_SECRET_TOKEN
// requires a process restart to take effect — there is no hot-reload. This is
// the desired behavior for an immutable, container-injected secret. (If runtime
// rotation were ever required, this pair could be swapped for an atomic.Pointer
// refreshed by a SIGHUP handler without touching the middleware logic below.)
var (
	// secretOnce guarantees the environment is read a single time.
	secretOnce sync.Once
	// secretValue holds the cached secret after the first read.
	secretValue string
)

// expectedSecret returns the configured shared secret, lazily reading
// MCP_SECRET_TOKEN from the environment on first call and returning the cached
// value thereafter. It is safe for concurrent use by many request goroutines.
//
// An empty return value means the secret is unset/misconfigured; callers must
// treat that as fail-closed (HTTP 500), never as "allow".
func expectedSecret() string {
	secretOnce.Do(func() {
		secretValue = os.Getenv(secretTokenEnvVar)
	})
	return secretValue
}

// The following constants centralize every "magic string" involved in the
// authentication handshake. Defining them once (rather than inline) prevents
// subtle typos, documents the wire contract in a single place, and makes the
// credential-extraction logic below trivially readable.
const (
	// secretTokenEnvVar is the name of the environment variable that holds the
	// server's expected shared secret. The value is injected at deploy time
	// (see the Phase 5 docker-compose / Nginx integration) and MUST NOT be
	// hard-coded or committed to source control.
	secretTokenEnvVar = "MCP_SECRET_TOKEN"

	// authorizationHeaderName is the canonical HTTP header a well-behaved MCP
	// client uses to present its credentials.
	authorizationHeaderName = "Authorization"

	// bearerSchemePrefix is the RFC 6750 scheme prefix that precedes the token
	// inside the Authorization header, e.g. "Authorization: Bearer <token>".
	// The trailing space is significant and intentional.
	bearerSchemePrefix = "Bearer "

	// tokenQueryParamName is the fallback query-string key. Certain MCP clients
	// cannot attach custom headers to the initial SSE handshake (a browser
	// EventSource, for example), so we allow the token to arrive as
	// "?token=<token>". The header channel always takes precedence over this.
	tokenQueryParamName = "token"
)

// TokenAuthMiddleware returns an http.Handler decorator that enforces
// shared-secret Bearer-token authentication on every request routed through it.
//
// # Why a decorator
//
// This middleware is designed to wrap the ENTIRE application mux at the
// composition root (cmd/server, Phase 3). Applying the security check at one
// single choke point — rather than sprinkling checks across individual handlers
// — guarantees that no endpoint (present or future) can be exposed without
// authentication. This is the "secure by default" posture demanded by the
// architecture: a developer must go out of their way to *remove* protection,
// never to *add* it.
//
// # Behavior contract
//
// The returned handler evaluates each request in the following order:
//
//  1. Load the expected secret from the MCP_SECRET_TOKEN environment variable.
//     If it is empty, the SERVER is misconfigured (not the client), so we
//     respond 500 Internal Server Error and log the fault. We never fall
//     through to "allow" on a missing secret — an unset secret must fail
//     closed, never open.
//  2. Extract the presented token from the request (Authorization header first,
//     then the `token` query parameter). If none is present, respond 401.
//  3. Compare the presented token against the expected secret using a
//     constant-time comparison to defeat timing attacks. On mismatch, respond
//     401. On match, delegate to the wrapped handler (next).
//
// The middleware deliberately keeps its client-facing responses terse and
// generic. It never echoes the presented token, and it never reveals whether
// the failure was due to a missing versus malformed versus incorrect token in a
// way that would aid credential-guessing — while still logging enough
// server-side detail for operators to debug.
func TokenAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// --- Step 1: Resolve the server-side expected secret. ---------------
		//
		// expectedSecret reads MCP_SECRET_TOKEN exactly once and caches it
		// (Audit O-2), so this is a lock-free read on the hot path after the
		// first request. An empty result means misconfiguration and is handled
		// fail-closed below.
		expectedToken := expectedSecret()
		if expectedToken == "" {
			// Fail closed. A missing secret is an operator error: refusing all
			// traffic is far safer than silently disabling authentication.
			log.Printf(
				"auth: refusing request to %q: %s is not set (server misconfiguration)",
				r.URL.Path, secretTokenEnvVar,
			)
			http.Error(w, "Server authentication is not configured.", http.StatusInternalServerError)
			return
		}

		// --- Step 2: Extract the client-presented token. --------------------
		presentedToken, source := extractPresentedToken(r)
		if presentedToken == "" {
			log.Printf(
				"auth: rejecting unauthenticated request to %q from %s: no credentials presented",
				r.URL.Path, r.RemoteAddr,
			)
			// WWW-Authenticate advertises the expected scheme so compliant
			// clients know exactly how to retry — a small self-healing hint at
			// the transport layer.
			w.Header().Set("WWW-Authenticate", `Bearer realm="Custom-VPS-MCP-Engine"`)
			http.Error(w, "Unauthorized: missing bearer token.", http.StatusUnauthorized)
			return
		}

		// --- Step 3: Constant-time secret comparison. -----------------------
		//
		// subtle.ConstantTimeCompare runs in time independent of where the
		// first differing byte occurs, which prevents an attacker from
		// recovering the secret one byte at a time by measuring response
		// latency. It returns 1 only when both length and content match.
		if subtle.ConstantTimeCompare([]byte(presentedToken), []byte(expectedToken)) != 1 {
			log.Printf(
				"auth: rejecting request to %q from %s: invalid token (source=%s)",
				r.URL.Path, r.RemoteAddr, source,
			)
			w.Header().Set("WWW-Authenticate", `Bearer realm="Custom-VPS-MCP-Engine", error="invalid_token"`)
			http.Error(w, "Unauthorized: invalid bearer token.", http.StatusUnauthorized)
			return
		}

		// Authentication succeeded — hand control to the protected handler.
		next.ServeHTTP(w, r)
	})
}

// extractPresentedToken pulls the client's credential out of an incoming
// request, honoring the two supported credential channels in priority order.
//
// It returns the raw token string and a short, log-friendly label describing
// which channel it came from ("header" or "query"). When no credential is
// found, it returns two empty strings, and the caller is responsible for
// emitting the appropriate 401.
//
// Priority rationale: the Authorization header is the standard, most secure
// channel (it is not logged by proxies the way full URLs often are), so it is
// always preferred. The query-parameter fallback exists purely for clients that
// cannot set headers on the SSE handshake, and is only consulted when the header
// is absent or malformed.
func extractPresentedToken(r *http.Request) (token string, source string) {
	// Channel 1 (preferred): "Authorization: Bearer <token>".
	rawHeader := r.Header.Get(authorizationHeaderName)
	if rawHeader != "" {
		// A case-insensitive scheme check keeps us lenient toward clients that
		// send "bearer" instead of "Bearer", while still requiring the scheme
		// to be present so we don't misinterpret some other auth scheme's value
		// as our token.
		if len(rawHeader) >= len(bearerSchemePrefix) &&
			strings.EqualFold(rawHeader[:len(bearerSchemePrefix)], bearerSchemePrefix) {
			// Trim surrounding whitespace to tolerate clients that add padding
			// around the token value.
			candidate := strings.TrimSpace(rawHeader[len(bearerSchemePrefix):])
			if candidate != "" {
				return candidate, "header"
			}
		}
		// Header was present but not a usable Bearer credential. We intentionally
		// fall through to the query-parameter channel rather than failing here,
		// so a client that sets an unrelated header can still authenticate via
		// the fallback.
	}

	// Channel 2 (fallback): "?token=<token>".
	if candidate := strings.TrimSpace(r.URL.Query().Get(tokenQueryParamName)); candidate != "" {
		return candidate, "query"
	}

	// No credential presented through either channel.
	return "", ""
}
