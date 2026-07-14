package auth

import "sync"

// This file is compiled ONLY during test builds (Go treats *_test.go files as
// test-only). It exposes internal test hooks so production code stays free of
// test-support scaffolding.

// resetSecretCacheForTest clears the process-wide, one-time secret cache created
// by sync.Once in middleware.go.
//
// The production code reads MCP_SECRET_TOKEN exactly once for performance
// (Audit O-2). Tests, however, deliberately vary that environment variable
// across cases, so they must be able to force a re-read. Calling this after
// t.Setenv makes the next expectedSecret() call observe the new value.
//
// It must only ever be called from tests.
func resetSecretCacheForTest() {
	secretOnce = sync.Once{}
	secretValue = ""
}
