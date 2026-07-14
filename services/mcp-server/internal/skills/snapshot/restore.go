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
	// toolNameRestore is the MCP tool identifier for restoring a codebase snapshot.
	toolNameRestore = "snapshot_restore"

	// defaultParentDir is the parent of the active services directory on the VPS.
	defaultParentDir = "/opt/micro-services.d"

	// servicesBackupPrefix is prepended to a timestamp when renaming the active
	// services directory before extraction (e.g. services.bak-2026-07-12_12-00-00).
	servicesBackupPrefix = "services.bak-"
)

// RestoreInput is the input schema for the snapshot_restore tool.
//
// Filename must be the basename of an archive in defaultDestDir, such as
// snapshot-2026-07-12_12-00-00.tar.gz. Path separators and parent-directory
// references are rejected.
type RestoreInput struct {
	// Filename is the snapshot archive basename to restore from defaultDestDir.
	Filename string `json:"filename" jsonschema:"Required. Basename of the snapshot archive to restore, for example 'snapshot-2026-07-12_12-00-00.tar.gz'. Must exist in /opt/micro-services.d/snapshots/."`
}

// RestoreResult summarizes a successful snapshot restoration.
type RestoreResult struct {
	ArchivePath string
	ServicesDir string
	BackupDir   string
}

// RestoreSnapshot extracts a validated archive from defaultDestDir into
// defaultSourceDir using a failsafe rename-then-extract workflow.
//
// Workflow:
//  1. Validate filename and confirm the archive exists.
//  2. Rename defaultSourceDir to services.bak-<timestamp> under defaultParentDir.
//  3. Create a fresh defaultSourceDir and extract the archive into it with tar -xzf.
//
// If extraction fails, the partial services directory is removed and the backup
// directory is renamed back to defaultSourceDir (rollback).
func RestoreSnapshot(ctx context.Context, filename string) (RestoreResult, error) {
	return restoreSnapshotAt(ctx, filename, defaultDestDir, defaultSourceDir, defaultParentDir)
}

// restoreSnapshotAt is the testable implementation of RestoreSnapshot.
func restoreSnapshotAt(ctx context.Context, filename, snapshotsDir, servicesDir, parentDir string) (RestoreResult, error) {
	snapshotsDir = filepath.Clean(snapshotsDir)
	servicesDir = filepath.Clean(servicesDir)
	parentDir = filepath.Clean(parentDir)

	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	if err := validateSnapshotFilename(filename); err != nil {
		return RestoreResult{}, fmt.Errorf("validate snapshot filename: %w", err)
	}

	archivePath := filepath.Join(snapshotsDir, filename)
	if err := validateArchiveFile(archivePath); err != nil {
		return RestoreResult{}, err
	}

	if err := validateSourceDir(servicesDir); err != nil {
		return RestoreResult{}, fmt.Errorf("pre-restore services directory: %w", err)
	}

	timestamp := time.Now().Format(snapshotTimeFormat)
	backupDir := filepath.Join(parentDir, servicesBackupPrefix+timestamp)

	if _, err := os.Stat(backupDir); err == nil {
		return RestoreResult{}, fmt.Errorf("backup path already exists: %q", backupDir)
	} else if !os.IsNotExist(err) {
		return RestoreResult{}, fmt.Errorf("stat backup path %q: %w", backupDir, err)
	}

	if err := os.Rename(servicesDir, backupDir); err != nil {
		return RestoreResult{}, fmt.Errorf("rename active services directory %q to backup %q: %w", servicesDir, backupDir, err)
	}

	extractedOK := false
	defer func() {
		if extractedOK {
			return
		}
		// The primary extraction error has already been captured for the caller;
		// the rollback below is a best-effort attempt to reinstate the
		// pre-restore tree. A rollback failure is CRITICAL operational
		// information — it means the VPS may be left with NO active services
		// directory at all — so it must never be silently discarded (core
		// directive: no swallowed errors). It cannot be joined into the already-
		// returned primary error from a deferred closure, so it is logged loudly
		// with explicit manual-recovery instructions instead.
		if rbErr := rollbackFailedRestore(servicesDir, backupDir); rbErr != nil {
			log.Printf(
				"skills/snapshot: CRITICAL: rollback after failed restore also failed: %v "+
					"(manual intervention required on the host: rename %q back to %q)",
				rbErr, backupDir, servicesDir,
			)
			return
		}
		log.Printf(
			"skills/snapshot: restore failed; pre-restore services tree rolled back from %q to %q",
			backupDir, servicesDir,
		)
	}()

	if err := os.Mkdir(servicesDir, destDirPerm); err != nil {
		return RestoreResult{}, fmt.Errorf("create fresh services directory %q: %w", servicesDir, err)
	}

	if err := extractSnapshotArchive(ctx, archivePath, servicesDir); err != nil {
		return RestoreResult{}, fmt.Errorf("extract snapshot archive %q: %w", archivePath, err)
	}

	extractedOK = true
	return RestoreResult{
		ArchivePath: archivePath,
		ServicesDir: servicesDir,
		BackupDir:   backupDir,
	}, nil
}

// validateSnapshotFilename rejects empty names, path traversal, and filenames
// outside the snapshot_create naming convention.
func validateSnapshotFilename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("filename must not be empty")
	}
	if name != filepath.Base(name) {
		return fmt.Errorf("filename %q must not contain path separators", name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("filename %q must not contain ..", name)
	}
	if !strings.HasPrefix(name, snapshotFilenamePrefix) {
		return fmt.Errorf("filename %q must start with %q", name, snapshotFilenamePrefix)
	}
	if !strings.HasSuffix(name, ".tar.gz") {
		return fmt.Errorf("filename %q must end with .tar.gz", name)
	}
	return nil
}

// validateArchiveFile confirms archivePath exists and is a regular file.
func validateArchiveFile(archivePath string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("snapshot archive %q: %w", archivePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("snapshot archive %q is a directory, want a regular file", archivePath)
	}
	return nil
}

// buildExtractTarArgs returns the fixed argument slice for extracting a gzip
// tarball into destDir. Archives produced by snapshot_create store paths
// relative to the services root (-C sourceDir .), so destDir must be the
// services directory itself.
//
// Example equivalent shell-free invocation:
//
//	tar -xzf <archivePath> -C <servicesDir>
func buildExtractTarArgs(archivePath, destDir string) []string {
	return []string{"-xzf", archivePath, "-C", destDir}
}

// extractSnapshotArchive runs tar to extract archivePath into destDir.
func extractSnapshotArchive(ctx context.Context, archivePath, destDir string) error {
	tarPath, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("locate tar binary: %w", err)
	}

	args := buildExtractTarArgs(archivePath, destDir)
	cmd := exec.CommandContext(ctx, tarPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run tar %v: %w (output: %s)", args, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// rollbackFailedRestore removes a partial servicesDir and renames backupDir back
// to servicesDir so the VPS retains the pre-restore codebase after a failed
// extraction.
func rollbackFailedRestore(servicesDir, backupDir string) error {
	if err := os.RemoveAll(servicesDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove partial services directory %q during rollback: %w", servicesDir, err)
	}
	if err := os.Rename(backupDir, servicesDir); err != nil {
		return fmt.Errorf("rename backup %q back to active services %q during rollback: %w", backupDir, servicesDir, err)
	}
	return nil
}

// handleSnapshotRestore implements the snapshot_restore MCP tool.
func handleSnapshotRestore(ctx context.Context, _ *mcp.CallToolRequest, in RestoreInput) (*mcp.CallToolResult, any, error) {
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		return errorResult(
			"Missing required 'filename' argument. Provide the snapshot archive basename to restore, "+
				"for example: \"snapshot-2026-07-12_12-00-00.tar.gz\". List available files in %s.",
			defaultDestDir,
		), nil, nil
	}

	result, err := RestoreSnapshot(ctx, filename)
	if err != nil {
		return errorResult("Failed to restore codebase snapshot: %v", err), nil, nil
	}

	report := fmt.Sprintf(
		"status: ok\n"+
			"restored_from: %s\n"+
			"services_dir: %s\n"+
			"previous_services_backup: %s\n"+
			"restored_at_utc: %s",
		result.ArchivePath,
		result.ServicesDir,
		result.BackupDir,
		time.Now().UTC().Format(time.RFC3339),
	)

	return textResult(report), nil, nil
}
