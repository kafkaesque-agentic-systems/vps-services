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
	//
	// ConnectTimeout bounds TCP connection establishment, which BatchMode does
	// NOT cover. Without it an unreachable or firewalled host (a filtered SSH
	// port, a stale UFW source-IP allowlist) leaves rsync blocking on connect
	// until rsyncTimeout, long past the point the MCP client has given up — the
	// caller sees only an opaque request timeout and a zero-byte ledger, with
	// the real cause reported nowhere. Ten seconds turns that silent multi-minute
	// hang into an immediate, actionable "Operation timed out".
	rsyncRemoteShell = "ssh -o BatchMode=yes -o ConnectTimeout=10"

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
//
// CRITICAL — snapshots/: since the 2026-07-14 layout migration the VPS
// snapshot store lives INSIDE the sync root (/opt/micro-services.d/snapshots).
// The local checkout has no snapshots directory, so WITHOUT this exclusion
// rsync --delete would erase every snapshot archive on the VPS on the very
// first push. An excluded path is protected from --delete as well as from
// transfer; never remove this entry while the store lives inside the root.
var rsyncExcludes = []string{
	".git/",
	"node_modules/",
	".venv/",
	"__pycache__/",
	".env",
	".environs",
	"image/",
	"vol/",
	"snapshots/",
	".bak-*", // pre-restore backups created by snapshot_restore inside the VPS tree

	// Any timestamped backup, wherever it sits. The ".bak-*" pattern above only
	// matches basenames BEGINNING with ".bak-", so it does not cover
	// ".env.bak-20260719" or "docker-compose.yml.bak-20260719". Those are
	// operator-made copies of live config; ".env.bak-*" in particular would
	// carry MCP_SECRET_TOKEN into the VPS tree and from there into every
	// subsequent snapshot archive.
	"*.bak-*",

	// Local build and runtime artifacts of the DEVELOPER's machine. The deploy
	// source is a macOS checkout, so "server" is a Darwin binary that is useless
	// on the Linux VPS and actively misleading sitting beside the real build;
	// "logs/" is the local LaunchAgent's output, which would otherwise be swept
	// into every VPS snapshot. Both paths are anchored with a leading slash so
	// they match only at the transfer root, never an unrelated nested "server"
	// or "logs" belonging to a service.
	"/mcp-server/server",
	"/mcp-server/logs/",

	// Compiled front-end output. Every service builds inside its own Docker
	// image, so a dist/ in the source tree is always a developer-machine
	// artifact -- and one built for the wrong platform at that. Shipping it
	// would put stale, unused bundles on the VPS and into every snapshot.
	"dist/",

	// The ledger store itself. Ledgers are a record OF deploys and must never be
	// shipped BY one, or each push copies the previous push's logs to the VPS.
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
		// Omit directory mtimes. -a implies -t, and setting a directory's mtime
		// requires OWNING it. The VPS sync root is owned by the container's mcp
		// user (uid 100 / systemd-network on the host) so that snapshot_create
		// can write from inside the container; the deploy user reaches it only
		// through group write, which permits creating entries but not
		// restamping the directory. Without this flag every push fails on
		// `failed to set times on "."` and exits 23, reporting the entire
		// deploy as failed even when all file content transferred correctly.
		// Chowning the root to the deploy user would silence it but break
		// snapshots -- the rollback safety net -- so the flag is the correct
		// trade: directory mtimes carry no deployment meaning, file times do.
		"--omit-dir-times",
		// Do not transfer file modes. -a implies -p, which makes the DEVELOPER's
		// machine authoritative over production permissions -- a local umask or
		// macOS default silently rewrites server modes on every push. That is
		// backwards: the server's permission model is deliberate and belongs to
		// the server. It bit us concretely on 2026-07-19, when pushing local
		// 0640/0750 modes onto files the container reads left snapshot_create
		// unable to tar the tree ("Cannot open: Permission denied"), destroying
		// the rollback safety net while every service still looked healthy.
		// Deploys carry CONTENT; the host owns permissions.
		"--no-perms",
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
