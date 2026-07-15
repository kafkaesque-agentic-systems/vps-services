package snapshot

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// toolNameCreate is the MCP tool identifier for creating a codebase snapshot.
	toolNameCreate = "snapshot_create"

	// defaultSourceDir is the VPS path to the active codebase tree that is
	// archived on each invocation.
	//
	// LAYOUT MIGRATION (2026-07-14): the codebase moved from
	// /opt/micro-services.d/services up to /opt/micro-services.d itself. The
	// snapshots directory therefore now lives INSIDE the source tree, which is
	// why "snapshots" appears in excludedDirNames below — without it every
	// archive would recursively contain all previous archives.
	defaultSourceDir = "/opt/micro-services.d"

	// defaultDestDir is the VPS path where snapshot archives are written.
	// CreateSnapshot ensures this directory exists via os.MkdirAll before
	// writing. NOTE: it is a child of defaultSourceDir (see layout migration
	// note above); restore.go contains dedicated handling for this nesting.
	defaultDestDir = "/opt/micro-services.d/snapshots"

	// snapshotTimeFormat produces human-readable, filesystem-safe timestamps such
	// as 2026-07-12_11-38-00 for archive filenames.
	snapshotTimeFormat = "2006-01-02_15-04-05"

	// snapshotFilenamePrefix is prepended to the formatted timestamp in archive names.
	snapshotFilenamePrefix = "snapshot-"

	// snapshotTimeout bounds how long a single tar invocation may run before the
	// context cancels it. Large trees may take several minutes; ten minutes is a
	// conservative upper bound for typical VPS deployments.
	snapshotTimeout = 10 * time.Minute

	// destDirPerm is applied when creating the snapshots directory.
	destDirPerm = 0o755
)

// excludedDirNames lists names (tar glob patterns) under defaultSourceDir that
// are omitted from every archive.
//
//   - image, vol: bulky persistent runtime assets (disk-space economy).
//   - snapshots:  the archive store itself, which lives INSIDE the source tree
//     since the 2026-07-14 layout migration. Excluding it is CORRECTNESS, not
//     economy — otherwise every snapshot would recursively swallow all prior
//     snapshots, growing without bound.
//   - .bak-*:     pre-restore backup directories created by snapshot_restore
//     inside the tree (see restore.go backupNamePrefix); archiving them would
//     snowball old codebase generations into every new snapshot.
var excludedDirNames = []string{"image", "vol", "snapshots", ".bak-*"}

// matchesExcludedName reports whether a top-level entry name matches one of
// the archive-exclusion patterns above (patterns use filepath.Match globs,
// e.g. ".bak-*").
//
// # Why restore preservation derives from the exclusion list
//
// Anything excluded from archives can never be re-created BY a restore. If the
// contents-swap in restore.go moved such an entry into the backup, the restored
// tree would silently lose it — for image/ and vol/ that means dropping
// persistent runtime assets from the active tree. Deriving the preservation
// set from THIS list keeps the two behaviors in lockstep by construction:
// excluded from archives ⇔ preserved in place across restores.
func matchesExcludedName(name string) bool {
	for _, pattern := range excludedDirNames {
		if ok, err := filepath.Match(pattern, name); err == nil && ok {
			return true
		}
	}
	return false
}

// SnapshotInput is the input schema for the snapshot_create tool.
//
// The tool intentionally takes NO parameters: snapshot paths and exclusions are
// fixed operational constants. An empty struct causes the SDK to generate a
// JSON schema describing an object with no properties.
type SnapshotInput struct{}

// Register attaches every tool owned by the snapshot skill to the provided MCP
// server.
//
// This is the single exported entry point of the package — the composition root
// (cmd/server) calls it via registerCustomSkills.
func Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameCreate,
		Description: "Creates a gzip-compressed tar archive of the VPS codebase at " +
			defaultSourceDir + ", writing the snapshot to " + defaultDestDir + ". " +
			"Excludes the top-level 'image', 'vol', and 'snapshots' directories plus '.bak-*' restore " +
			"backups (the snapshots exclusion prevents recursive self-archiving, as the store lives " +
			"inside the codebase tree). The output filename uses the format " +
			"snapshot-YYYY-MM-DD_HH-MM-SS.tar.gz. Takes no arguments. Use before applying major " +
			"AI-driven changes.",
	}, handleSnapshotCreate)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameRestore,
		Description: "Restores the VPS codebase from a snapshot archive in " +
			defaultDestDir + " into " + defaultSourceDir + ". " +
			"Requires the 'filename' argument (archive basename, e.g. snapshot-2026-07-12_12-00-00.tar.gz). " +
			"Contents-swap failsafe: moves the current tree contents into a " + backupNamePrefix +
			"<timestamp> backup directory INSIDE the tree (the root itself is a container bind mount and " +
			"cannot be renamed), preserves the snapshots store in place, then extracts; rolls back " +
			"automatically if extraction fails. Archives created before the 2026-07-14 layout migration " +
			"(codebase under /opt/micro-services.d/services) are NOT compatible.",
	}, handleSnapshotRestore)

	log.Printf("skills/snapshot: registered tools %q, %q", toolNameCreate, toolNameRestore)
}

// CreateSnapshot archives defaultSourceDir into a timestamped .tar.gz file under
// defaultDestDir.
//
// It ensures defaultDestDir exists, validates that the source is a directory,
// and invokes tar via os/exec with fixed arguments (no shell). The two
// top-level directories image and vol are excluded from the archive.
//
// Returns the absolute path to the created archive on success.
func CreateSnapshot(ctx context.Context) (string, error) {
	return createSnapshotAt(ctx, defaultSourceDir, defaultDestDir)
}

// createSnapshotAt archives sourceDir into a timestamped .tar.gz under destDir.
// It is the testable implementation used by CreateSnapshot and unit tests.
func createSnapshotAt(ctx context.Context, sourceDir, destDir string) (string, error) {
	sourceDir = filepath.Clean(sourceDir)
	destDir = filepath.Clean(destDir)

	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	if err := validateSourceDir(sourceDir); err != nil {
		return "", err
	}

	if err := os.MkdirAll(destDir, destDirPerm); err != nil {
		return "", fmt.Errorf("create snapshot destination directory %q: %w", destDir, err)
	}

	timestamp := time.Now().Format(snapshotTimeFormat)
	destPath := filepath.Join(destDir, snapshotFilenamePrefix+timestamp+".tar.gz")

	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf("snapshot archive already exists: %q", destPath)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat snapshot destination %q: %w", destPath, err)
	}

	tarPath, err := exec.LookPath("tar")
	if err != nil {
		return "", fmt.Errorf("locate tar binary: %w", err)
	}

	args := buildTarArgs(destPath, sourceDir)
	cmd := exec.CommandContext(ctx, tarPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("run tar %v: %w (output: %s)", args, err, strings.TrimSpace(string(output)))
	}

	return destPath, nil
}

// validateSourceDir confirms sourceDir exists and is a directory.
func validateSourceDir(sourceDir string) error {
	info, err := os.Stat(sourceDir)
	if err != nil {
		return fmt.Errorf("source directory %q: %w", sourceDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path %q is not a directory", sourceDir)
	}
	return nil
}

// buildTarArgs returns the fixed argument slice for creating a gzip tarball of
// sourceDir while excluding image and vol relative to that root.
//
// Example equivalent shell-free invocation:
//
//	tar -czf <destPath> --exclude=image --exclude=vol -C <sourceDir> .
func buildTarArgs(destPath, sourceDir string) []string {
	args := []string{"-czf", destPath}
	for _, name := range excludedDirNames {
		args = append(args, "--exclude="+name)
	}
	args = append(args, "-C", sourceDir, ".")
	return args
}

// handleSnapshotCreate implements the snapshot_create MCP tool.
func handleSnapshotCreate(ctx context.Context, _ *mcp.CallToolRequest, _ SnapshotInput) (*mcp.CallToolResult, any, error) {
	archivePath, err := CreateSnapshot(ctx)
	if err != nil {
		return errorResult("Failed to create codebase snapshot: %v", err), nil, nil
	}

	report := fmt.Sprintf(
		"status: ok\n"+
			"archive: %s\n"+
			"source: %s\n"+
			"excluded: %s\n"+
			"created_at_utc: %s",
		archivePath,
		defaultSourceDir,
		strings.Join(excludedDirNames, ", "),
		time.Now().UTC().Format(time.RFC3339),
	)

	return textResult(report), nil, nil
}

// textResult builds a successful tool result carrying a single block of text.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// errorResult builds a tool result flagged as an error (IsError: true) whose
// text is a descriptive, self-healing message intended for the calling LLM.
func errorResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
	}
}
