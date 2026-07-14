package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ledgerDirName is the directory (under the local repo root) that collects
	// itemized deploy ledgers. It is both gitignored and rsync-excluded.
	ledgerDirName = "deploy_ledgers"

	// ledgerTimeFormat produces filesystem-safe ledger timestamps, mirroring the
	// snapshot skill's archive naming convention.
	ledgerTimeFormat = "2006-01-02_15-04-05"

	// ledgerDirPerm is applied when creating the deploy_ledgers directory.
	ledgerDirPerm = 0o755

	// rsyncRemoteShell is passed to rsync's -e flag as a single argument; rsync
	// tokenizes the command string itself, so no shell is involved. BatchMode=yes
	// makes ssh fail IMMEDIATELY when key authentication is unavailable instead
	// of blocking forever on an interactive password prompt — an MCP tool handler
	// is non-interactive by definition, so a prompt could only ever hang the call.
	rsyncRemoteShell = "ssh -o BatchMode=yes"

	// rsyncTimeout bounds a single push_codebase sync. Thirty minutes is a
	// generous ceiling for a full-tree first sync over a slow uplink while still
	// guaranteeing the tool call can never hang indefinitely (e.g. a stalled TCP
	// connection). Combined with BatchMode above, every failure mode of the
	// transport terminates deterministically.
	rsyncTimeout = 30 * time.Minute
)

// rsyncExcludes lists path patterns omitted from every push_codebase sync.
// These protect secrets, local-only artifacts, persistent VPS runtime data,
// and the local deployment ledger directory itself.
var rsyncExcludes = []string{
	".git/",
	"node_modules/",
	".venv/",
	"__pycache__/",
	".env",
	".environs",
	"image/",
	"vol/",
	"deploy_ledgers/",
}

// rsyncResult captures the outcome of a push_codebase rsync invocation.
type rsyncResult struct {
	// ledgerPath is the absolute path of the itemized ledger written by
	// rsync's --log-file under <localRoot>/deploy_ledgers/.
	ledgerPath string

	// ledgerText is the full trimmed content of that ledger file.
	ledgerText string

	// commandOutput is rsync's combined stdout/stderr, kept as a fallback for
	// the tool report when the ledger is empty.
	commandOutput string
}

// ledgerPathFor returns the absolute path to a timestamped deploy ledger file
// under <localRoot>/deploy_ledgers/.
func ledgerPathFor(localRoot string, now time.Time) string {
	filename := "deploy-" + now.UTC().Format(ledgerTimeFormat) + ".log"
	return filepath.Join(localRoot, ledgerDirName, filename)
}

// buildRsyncArgs constructs the argument slice for rsync push_codebase.
//
// The source path uses a trailing slash so rsync copies directory contents
// into the remote root rather than nesting an extra directory level.
func buildRsyncArgs(localRoot, remoteTarget, ledgerPath string) []string {
	source := ensureTrailingSlash(filepath.Clean(localRoot))
	dest := remoteTarget

	args := []string{
		"-a",
		"-z",
		"--delete",
		"-i",
		"--log-file=" + ledgerPath,
	}
	for _, pattern := range rsyncExcludes {
		args = append(args, "--exclude="+pattern)
	}
	args = append(args, "-e", rsyncRemoteShell, source, dest)
	return args
}

// runRsync executes rsync with push_codebase settings and returns the itemized
// ledger written to deploy_ledgers/.
//
// The invocation is bounded by rsyncTimeout (derived from the caller's ctx, so
// client cancellation and server shutdown still propagate first), matching the
// timeout discipline of every other exec-based skill in this server.
func runRsync(ctx context.Context, cfg *deployConfig) (*rsyncResult, error) {
	ctx, cancel := context.WithTimeout(ctx, rsyncTimeout)
	defer cancel()

	now := time.Now()
	ledgerPath := ledgerPathFor(cfg.localRoot, now)

	ledgerDir := filepath.Dir(ledgerPath)
	if err := os.MkdirAll(ledgerDir, ledgerDirPerm); err != nil {
		return nil, fmt.Errorf("create deploy ledger directory %q: %w", ledgerDir, err)
	}

	rsyncPath, err := exec.LookPath("rsync")
	if err != nil {
		return nil, fmt.Errorf("locate rsync binary: %w", err)
	}

	remoteDest := cfg.sshTarget + ":" + cfg.remotePath
	args := buildRsyncArgs(cfg.localRoot, remoteDest, ledgerPath)

	cmd := exec.CommandContext(ctx, rsyncPath, args...)
	output, err := cmd.CombinedOutput()
	combined := strings.TrimSpace(string(output))
	if err != nil {
		return nil, fmt.Errorf("run rsync %v: %w (output: %s)", args, err, combined)
	}

	ledgerBytes, readErr := os.ReadFile(ledgerPath)
	if readErr != nil {
		return nil, fmt.Errorf("read deploy ledger %q: %w", ledgerPath, readErr)
	}

	return &rsyncResult{
		ledgerPath:    ledgerPath,
		ledgerText:    strings.TrimSpace(string(ledgerBytes)),
		commandOutput: combined,
	}, nil
}
