package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// testToken is the shared secret used throughout these tests. It is an arbitrary
// non-empty string; the middleware's contract is that the presented credential
// must match this value exactly.
const testToken = "s3cr3t-test-token"

// TestTokenAuthMiddleware exercises the full decision matrix of
// TokenAuthMiddleware via a table-driven test.
//
// # Why table-driven
//
// The middleware's behavior is a small, well-defined truth table over three
// inputs (server secret present?, credential channel, credential validity). A
// table makes each row a self-documenting specification of one branch, and makes
// it trivial to add a new case later without duplicating the request/response
// plumbing.
//
// # Why not t.Parallel
//
// Every case manipulates the process-global MCP_SECRET_TOKEN environment
// variable via t.Setenv, which is fundamentally incompatible with parallel
// subtests (the Go test runtime forbids t.Setenv in a parallel test). We
// therefore run the rows sequentially; each is fast, so this costs nothing.
func TestTokenAuthMiddleware(t *testing.T) {
	tests := []struct {
		name string

		// envToken is the value assigned to MCP_SECRET_TOKEN for the case. An
		// empty string models the "secret not configured" scenario, because the
		// middleware treats an empty secret identically to an unset one.
		envToken string

		// authHeader is the raw Authorization header value to send. Empty means
		// the header is omitted entirely.
		authHeader string

		// queryToken is the value of the ?token= fallback query parameter. Empty
		// means the parameter is omitted.
		queryToken string

		// wantStatus is the HTTP status code the middleware must return.
		wantStatus int

		// wantNextCalled asserts whether the wrapped (protected) handler should
		// have been reached. It must be true only when authentication succeeds.
		wantNextCalled bool
	}{
		{
			name:           "missing server secret yields 500 and blocks the handler",
			envToken:       "", // secret not configured -> fail closed
			authHeader:     "Bearer " + testToken,
			queryToken:     "",
			wantStatus:     http.StatusInternalServerError,
			wantNextCalled: false,
		},
		{
			name:           "no credentials yields 401",
			envToken:       testToken,
			authHeader:     "",
			queryToken:     "",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "invalid bearer token yields 401",
			envToken:       testToken,
			authHeader:     "Bearer not-the-right-token",
			queryToken:     "",
			wantStatus:     http.StatusUnauthorized,
			wantNextCalled: false,
		},
		{
			name:           "valid bearer header yields 200 and reaches the handler",
			envToken:       testToken,
			authHeader:     "Bearer " + testToken,
			queryToken:     "",
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
		{
			name:           "valid query-parameter token fallback yields 200",
			envToken:       testToken,
			authHeader:     "", // no header; must succeed via ?token= fallback
			queryToken:     testToken,
			wantStatus:     http.StatusOK,
			wantNextCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configure the server-side secret for this case. t.Setenv restores
			// the previous value automatically at the end of the subtest.
			t.Setenv(secretTokenEnvVar, tt.envToken)

			// The middleware caches the secret via sync.Once (Audit O-2), so we
			// must clear that cache between cases for the newly-set env value to
			// be observed. Subtests run sequentially, so this is race-free.
			resetSecretCacheForTest()

			// The protected handler simply records that it was reached and
			// returns 200. If the middleware rejects the request, this must not
			// run — that is exactly what wantNextCalled verifies.
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			handler := TokenAuthMiddleware(next)

			// Build the request. The path is arbitrary (auth is path-agnostic);
			// we append the token query parameter only when the case provides one.
			target := "/sse"
			if tt.queryToken != "" {
				target += "?" + tokenQueryParamName + "=" + tt.queryToken
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			if tt.authHeader != "" {
				req.Header.Set(authorizationHeaderName, tt.authHeader)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if nextCalled != tt.wantNextCalled {
				t.Errorf("next handler called = %v, want %v", nextCalled, tt.wantNextCalled)
			}
		})
	}
}
