package deploy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildRsyncArgs(t *testing.T) {
	localRoot := "/tmp/services"
	remoteTarget := "deploy@vps.example:/opt/micro-services.d/services/"
	ledgerPath := "/tmp/services/deploy_ledgers/deploy-2026-07-12_18-00-00.log"

	args := buildRsyncArgs(localRoot, remoteTarget, ledgerPath)

	wantPrefix := []string{"-a", "-z", "--delete", "-i", "--omit-dir-times", "--no-perms", "--log-file=" + ledgerPath}
	for i, want := range wantPrefix {
		if args[i] != want {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want)
		}
	}

	for _, pattern := range rsyncExcludes {
		found := false
		for _, arg := range args {
			if arg == "--exclude="+pattern {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing exclude %q in args: %v", pattern, args)
		}
	}

	sourceIdx := indexOf(args, "-e") + 1
	if sourceIdx <= 0 || sourceIdx >= len(args)-1 {
		t.Fatalf("expected -e <remote-shell> before source/dest, got %v", args)
	}
	if args[sourceIdx-1] != "-e" || args[sourceIdx] != rsyncRemoteShell {
		t.Errorf("remote shell = %v, want -e %q", args[sourceIdx-1:sourceIdx+1], rsyncRemoteShell)
	}
	// Guard: the remote shell MUST be non-interactive. Without BatchMode=yes a
	// failed key auth would block the MCP handler on a password prompt forever.
	if !strings.Contains(rsyncRemoteShell, "BatchMode=yes") {
		t.Errorf("rsyncRemoteShell = %q, must enforce ssh -o BatchMode=yes", rsyncRemoteShell)
	}
	// Guard: BatchMode bounds AUTHENTICATION but not CONNECTION. Without an
	// explicit ConnectTimeout an unreachable or firewalled host leaves rsync
	// blocking on connect until the 30-minute ceiling, long after the MCP client
	// has timed out, surfacing as an opaque failure with a zero-byte ledger.
	if !strings.Contains(rsyncRemoteShell, "ConnectTimeout=") {
		t.Errorf("rsyncRemoteShell = %q, must set ssh -o ConnectTimeout=", rsyncRemoteShell)
	}
	// Guard: the deploy must never rewrite production file modes from the
	// developer's machine. Pushing local 0640/0750 modes onto files the mcp
	// container reads broke snapshot_create on 2026-07-19 -- the rollback
	// safety net -- while every service still appeared healthy.
	if indexOf(args, "--no-perms") < 0 {
		t.Errorf("args must contain --no-perms so the host owns permissions: %v", args)
	}
	// Guard: directory mtimes cannot be set on a sync root the deploy user does
	// not own, and that single failure fails the ENTIRE push with exit 23.
	if indexOf(args, "--omit-dir-times") < 0 {
		t.Errorf("args must contain --omit-dir-times: %v", args)
	}

	source := args[len(args)-2]
	dest := args[len(args)-1]
	if source != "/tmp/services/" {
		t.Errorf("source = %q, want trailing-slash local root", source)
	}
	if dest != remoteTarget {
		t.Errorf("dest = %q, want %q", dest, remoteTarget)
	}
}

// TestRsyncExcludesContainsRequiredPatterns pins the exclusion list itself.
//
// TestBuildRsyncArgs only verifies that whatever rsyncExcludes contains reaches
// the command line, so silently DELETING an entry from the list keeps that test
// green. Each pattern below prevents a specific, observed failure, so the set is
// asserted directly.
func TestRsyncExcludesContainsRequiredPatterns(t *testing.T) {
	required := map[string]string{
		".git/":              "version control metadata is not deployment content",
		".env":               "carries MCP_SECRET_TOKEN and DEPLOY_SSH_TARGET",
		".environs":          "host-specific runtime config, owned by the VPS",
		"vol/":               "database volume data; syncing it would overwrite live storage",
		"image/":             "build artifacts",
		"snapshots/":         "the rollback store lives inside the sync root; --delete would erase it",
		".bak-*":             "snapshot_restore's pre-restore backups on the VPS",
		"*.bak-*":            "operator copies such as .env.bak-<ts>, which carry secrets",
		"/mcp-server/server": "a Darwin build artifact, meaningless on the Linux VPS",
		"/mcp-server/logs/":  "local LaunchAgent logs would be swept into VPS snapshots",
		"deploy_ledgers/":    "ledgers record deploys and must not be shipped by one",
		"dist/":              "front-end build output is produced inside Docker, never shipped",
	}

	present := make(map[string]bool, len(rsyncExcludes))
	for _, p := range rsyncExcludes {
		present[p] = true
	}

	for pattern, why := range required {
		if !present[pattern] {
			t.Errorf("rsyncExcludes is missing %q — %s", pattern, why)
		}
	}
}

func TestLedgerPathFormat(t *testing.T) {
	now := time.Date(2026, 7, 12, 18, 30, 45, 0, time.UTC)
	got := ledgerPathFor("/repo/root", now)
	want := filepath.Join("/repo/root", "deploy_ledgers", "deploy-2026-07-12_18-30-45.log")
	if got != want {
		t.Errorf("ledgerPathFor() = %q, want %q", got, want)
	}
}

func TestResolveDeployConfigRequiresSSHTarget(t *testing.T) {
	t.Setenv(envDeploySSHTarget, "")
	t.Setenv(envMCPSecretToken, "secret")

	_, err := resolveDeployConfig()
	if err == nil {
		t.Fatal("resolveDeployConfig without DEPLOY_SSH_TARGET: want error")
	}
	if !strings.Contains(err.Error(), envDeploySSHTarget) {
		t.Errorf("error = %q, want mention of %s", err, envDeploySSHTarget)
	}
}

func TestResolveDeployConfigRequiresToken(t *testing.T) {
	t.Setenv(envDeploySSHTarget, "deploy@vps")
	t.Setenv(envMCPSecretToken, "")

	_, err := resolveDeployConfig()
	if err == nil {
		t.Fatal("resolveDeployConfig without MCP_SECRET_TOKEN: want error")
	}
	if !strings.Contains(err.Error(), envMCPSecretToken) {
		t.Errorf("error = %q, want mention of %s", err, envMCPSecretToken)
	}
}

func TestResolveDeployConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, composeMarker), []byte("services: {}"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	t.Setenv(envDeploySSHTarget, "deploy@vps")
	t.Setenv(envMCPSecretToken, "secret")
	t.Setenv(envDeployRemotePath, "")
	t.Setenv(envDeployMCPURL, "")
	t.Setenv(envDeployLocalRoot, dir)

	cfg, err := resolveDeployConfig()
	if err != nil {
		t.Fatalf("resolveDeployConfig: %v", err)
	}

	if cfg.remotePath != defaultRemotePath {
		t.Errorf("remotePath = %q, want %q", cfg.remotePath, defaultRemotePath)
	}
	if cfg.mcpURL != defaultMCPURL {
		t.Errorf("mcpURL = %q, want %q", cfg.mcpURL, defaultMCPURL)
	}
	if cfg.localRoot != filepath.Clean(dir) {
		t.Errorf("localRoot = %q, want %q", cfg.localRoot, filepath.Clean(dir))
	}
}

func TestDetectLocalRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, composeMarker), []byte("services: {}"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	sub := filepath.Join(dir, "mcp-server")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	t.Chdir(sub)

	got, err := detectLocalRoot()
	if err != nil {
		t.Fatalf("detectLocalRoot: %v", err)
	}
	if got != filepath.Clean(dir) {
		t.Errorf("detectLocalRoot() = %q, want %q", got, filepath.Clean(dir))
	}
}

func TestBearerRoundTripper(t *testing.T) {
	const token = "test-secret-token"
	var seenAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newBearerHTTPClient(token)
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if seenAuth != "Bearer "+token {
		t.Errorf("Authorization header = %q, want %q", seenAuth, "Bearer "+token)
	}
}

func TestEnsureTrailingSlash(t *testing.T) {
	if got := ensureTrailingSlash("/opt/path"); got != "/opt/path/" {
		t.Errorf("ensureTrailingSlash = %q", got)
	}
	if got := ensureTrailingSlash("/opt/path/"); got != "/opt/path/" {
		t.Errorf("ensureTrailingSlash already slashed = %q", got)
	}
}

func indexOf(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}
