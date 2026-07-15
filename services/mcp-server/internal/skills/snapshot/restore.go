package snapshot

import (
	"context"
	"fmt"
	"io/fs"
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

	// backupNamePrefix names pre-restore backup directories created INSIDE the
	// codebase tree (e.g. /opt/micro-services.d/.bak-2026-07-14_12-00-00).
	//
	// WHY INSIDE THE TREE — this is load-bearing, not cosmetic:
	//
	//  1. MOUNTPOINT CONSTRAINT: in production the codebase root
	//     /opt/micro-services.d IS the go-mcp container's bind-mount point.
	//     Renaming a mountpoint from inside a container fails with EBUSY, so
	//     the classic "rename the whole tree aside" failsafe is impossible.
	//     Every operation below is a rename WITHIN the mount, which is legal.
	//  2. ATOMICITY: keeping source and destination on one filesystem makes
	//     each per-entry os.Rename atomic (no cross-device copy fallback).
	//
	// The dot prefix keeps backups visually out of the way; snapshot_create
	// excludes ".bak-*" so archives never swallow backups, and push_codebase
	// excludes the same pattern so rsync --delete never erases them.
	backupNamePrefix = ".bak-"
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
	// ArchivePath is the absolute path of the archive that was extracted.
	ArchivePath string

	// CodebaseDir is the restored active tree (defaultSourceDir in production).
	CodebaseDir string

	// BackupDir holds the pre-restore contents, INSIDE the codebase tree.
	BackupDir string

	// ReclaimedAssets lists backup-relative paths of NESTED archive-excluded
	// entries moved back into the restored tree after extraction (e.g.
	// "prx/image"). Top-level excluded entries never move at all and are not
	// listed here.
	ReclaimedAssets []string

	// ReclaimWarnings carries non-fatal problems from the reclaim pass. The
	// restore itself succeeded and nothing was lost — affected entries remain
	// safely inside BackupDir and must be moved back manually.
	ReclaimWarnings []string
}

// RestoreSnapshot restores a validated archive from defaultDestDir into
// defaultSourceDir using a failsafe CONTENTS-SWAP workflow.
//
// # Workflow
//
//  1. Validate the filename and confirm the archive exists.
//  2. Create the backup directory .bak-<timestamp> inside the codebase tree.
//  3. Move every top-level entry of the tree into the backup — EXCEPT the
//     preserved set: the snapshots store (stays exactly where it is, keeping
//     the archive readable and every prior archive active), image/ and vol/
//     (persistent runtime assets excluded from archives — a restore cannot
//     re-create them, so they must remain live), and other .bak-* directories.
//  4. Extract the archive into the swapped-out tree. Archives never contain
//     any preserved name (see excludedDirNames), so extraction cannot collide
//     with what stayed behind.
//  5. Reclaim NESTED archive-excluded assets (e.g. prx/image) from the backup:
//     tar exclusions match at any depth, so such entries were swapped out with
//     their parent but cannot be re-created by extraction — they are moved
//     back wherever the restored tree lacks them (see reclaimExcludedAssets).
//
// If extraction fails, rollback removes whatever was partially extracted and
// moves the original contents back out of the backup. Rollback failures are
// logged loudly with manual-recovery instructions — never swallowed. Reclaim
// failures after a successful extraction are warnings, not errors: the assets
// remain safe inside the backup.
func RestoreSnapshot(ctx context.Context, filename string) (RestoreResult, error) {
	return restoreSnapshotAt(ctx, filename, defaultDestDir, defaultSourceDir)
}

// restoreSnapshotAt is the testable implementation of RestoreSnapshot. It
// handles the snapshots store living either inside the codebase tree
// (production, post-2026-07-14 layout) or outside it (legacy layout / tests).
func restoreSnapshotAt(ctx context.Context, filename, snapshotsDir, codebaseDir string) (RestoreResult, error) {
	snapshotsDir = filepath.Clean(snapshotsDir)
	codebaseDir = filepath.Clean(codebaseDir)

	ctx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	if err := validateSnapshotFilename(filename); err != nil {
		return RestoreResult{}, fmt.Errorf("validate snapshot filename: %w", err)
	}

	archivePath := filepath.Join(snapshotsDir, filename)
	if err := validateArchiveFile(archivePath); err != nil {
		return RestoreResult{}, err
	}

	if err := validateSourceDir(codebaseDir); err != nil {
		return RestoreResult{}, fmt.Errorf("pre-restore codebase directory: %w", err)
	}

	// Determine which top-level name (if any) shields the nested snapshots
	// store from the swap. rel is "snapshots" in production.
	nestedTop, err := nestedSnapshotsTopComponent(codebaseDir, snapshotsDir)
	if err != nil {
		return RestoreResult{}, err
	}

	// preserved reports whether a top-level entry must survive the swap
	// untouched:
	//
	//   - every backup directory (previous restores' and this one's);
	//   - the nested snapshots store (keeps the archive readable mid-restore);
	//   - EVERY name matching the archive-exclusion patterns (image, vol,
	//     snapshots, .bak-*): these can never be re-created by extraction, so
	//     moving them into the backup would silently drop persistent runtime
	//     assets from the restored tree (see matchesExcludedName).
	preserved := func(name string) bool {
		if strings.HasPrefix(name, backupNamePrefix) {
			return true
		}
		if nestedTop != "" && name == nestedTop {
			return true
		}
		return matchesExcludedName(name)
	}

	timestamp := time.Now().Format(snapshotTimeFormat)
	backupDir := filepath.Join(codebaseDir, backupNamePrefix+timestamp)

	// os.Mkdir (not MkdirAll) fails if the path exists — the collision check.
	if err := os.Mkdir(backupDir, destDirPerm); err != nil {
		return RestoreResult{}, fmt.Errorf("create pre-restore backup directory %q: %w", backupDir, err)
	}

	// From here on, any failure must trigger the rollback.
	extractedOK := false
	defer func() {
		if extractedOK {
			return
		}
		rollbackFailedRestore(codebaseDir, backupDir, preserved)
	}()

	// Step 3: move the current contents into the backup.
	entries, err := os.ReadDir(codebaseDir)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("list codebase directory %q: %w", codebaseDir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if preserved(name) {
			continue
		}
		if err := os.Rename(filepath.Join(codebaseDir, name), filepath.Join(backupDir, name)); err != nil {
			return RestoreResult{}, fmt.Errorf(
				"move %q into pre-restore backup %q: %w", name, backupDir, err,
			)
		}
	}

	// Step 4: extract. The archive path is untouched — the snapshots store
	// never moved.
	if err := extractSnapshotArchive(ctx, archivePath, codebaseDir); err != nil {
		return RestoreResult{}, fmt.Errorf("extract snapshot archive %q: %w", archivePath, err)
	}

	// Extraction succeeded — the rollback is disarmed BEFORE the reclaim pass,
	// because reclaim failures are non-destructive (affected entries simply
	// remain in the backup) and must never undo a good restore.
	extractedOK = true

	// Step 5: reclaim NESTED archive-excluded assets. tar exclusion patterns
	// match at any path depth, so entries like prx/image are absent from the
	// archive but were swapped into the backup along with their parent
	// directory. Move each one back wherever the restored tree lacks it.
	reclaimed, warnings := reclaimExcludedAssets(backupDir, codebaseDir)
	for _, w := range warnings {
		log.Printf("skills/snapshot: WARNING: reclaim after restore: %s", w)
	}

	return RestoreResult{
		ArchivePath:     archivePath,
		CodebaseDir:     codebaseDir,
		BackupDir:       backupDir,
		ReclaimedAssets: reclaimed,
		ReclaimWarnings: warnings,
	}, nil
}

// reclaimExcludedAssets walks the backup tree and moves back every entry whose
// NAME matches an archive-exclusion pattern (image, vol, snapshots, .bak-*)
// and whose counterpart is absent from the restored tree.
//
// # Why this pass exists
//
// tar's --exclude matches path components at ANY depth, so nested runtime
// assets (production example: prx/image, the proxy's static files) are never
// archived — but unlike top-level excluded entries, they cannot be preserved
// in place: their PARENT directory legitimately participates in the
// contents-swap. Without this pass a restore would strand them inside
// .bak-<timestamp> and the active tree would silently lose them.
//
// # Failure philosophy
//
// The pass is strictly best-effort and runs only after a successful
// extraction. Every failure is reported as a warning (returned AND logged by
// the caller), never as a restore error: the asset in question still exists,
// untouched, inside the backup — a warning tells the operator exactly what to
// move back manually, whereas failing the whole restore would help nobody.
//
// WalkDir visits entries in lexical order, so the reclaimed list is
// deterministic. Reclaimed directories are not descended into (their whole
// subtree moved with them); directories whose counterpart already exists are
// skipped rather than merged.
func reclaimExcludedAssets(backupDir, codebaseDir string) (reclaimed []string, warnings []string) {
	walkErr := filepath.WalkDir(backupDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("walk %q: %v", path, err))
			return nil // best-effort: keep walking what remains
		}
		if path == backupDir {
			return nil
		}
		if !matchesExcludedName(d.Name()) {
			return nil
		}

		rel, relErr := filepath.Rel(backupDir, path)
		if relErr != nil {
			warnings = append(warnings, fmt.Sprintf("relativize %q: %v", path, relErr))
			return nil
		}
		target := filepath.Join(codebaseDir, rel)

		if _, statErr := os.Lstat(target); statErr == nil {
			// Counterpart already present in the restored tree — leave the
			// backup copy where it is (no merging; the backup stays intact).
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		} else if !os.IsNotExist(statErr) {
			warnings = append(warnings, fmt.Sprintf("stat reclaim target %q: %v", target, statErr))
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if mkErr := os.MkdirAll(filepath.Dir(target), destDirPerm); mkErr != nil {
			warnings = append(warnings, fmt.Sprintf(
				"create parent for reclaim target %q: %v (entry remains in backup at %q)", target, mkErr, path))
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if mvErr := os.Rename(path, target); mvErr != nil {
			warnings = append(warnings, fmt.Sprintf(
				"move %q back to %q: %v (entry remains in backup)", path, target, mvErr))
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		reclaimed = append(reclaimed, rel)
		if d.IsDir() {
			return fs.SkipDir // the whole subtree moved with the rename
		}
		return nil
	})
	if walkErr != nil {
		warnings = append(warnings, fmt.Sprintf("walk backup %q: %v", backupDir, walkErr))
	}
	return reclaimed, warnings
}

// nestedSnapshotsTopComponent returns the FIRST path component of the
// snapshots store relative to the codebase tree when the store is nested
// inside it ("snapshots" in production), or "" when the store lives outside
// the tree (legacy layout / tests). Identical paths are a fatal
// misconfiguration.
func nestedSnapshotsTopComponent(codebaseDir, snapshotsDir string) (string, error) {
	rel, err := filepath.Rel(codebaseDir, snapshotsDir)
	if err != nil {
		// Unrelatable paths (e.g. different volumes) — treat as external.
		return "", nil
	}
	if rel == "." {
		return "", fmt.Errorf(
			"snapshots directory %q must not be the codebase directory itself", snapshotsDir,
		)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", nil
	}
	// First component shields the whole nested subtree from the swap.
	parts := strings.Split(rel, string(filepath.Separator))
	return parts[0], nil
}

// rollbackFailedRestore reverses a failed contents-swap restore:
//
//  1. Delete every top-level entry that is neither preserved nor the backup
//     itself — i.e. whatever a partial extraction deposited.
//  2. Move the original contents back out of the backup.
//  3. Remove the (now empty) backup directory.
//
// Preserved entries (the snapshots store, other .bak-* directories) are never
// touched, so archives can NEVER be destroyed by a rollback. Every failure is
// logged at CRITICAL with manual-recovery instructions (core directive: no
// swallowed errors).
func rollbackFailedRestore(codebaseDir, backupDir string, preserved func(string) bool) {
	backupName := filepath.Base(backupDir)

	entries, err := os.ReadDir(codebaseDir)
	if err != nil {
		log.Printf(
			"skills/snapshot: CRITICAL: rollback could not list %q: %v "+
				"(manual recovery: remove partially extracted files, then move the contents of %q back up one level)",
			codebaseDir, err, backupDir,
		)
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == backupName || preserved(name) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(codebaseDir, name)); err != nil {
			log.Printf(
				"skills/snapshot: CRITICAL: rollback could not remove partially extracted %q: %v "+
					"(manual recovery: remove it, then move the contents of %q back up one level)",
				filepath.Join(codebaseDir, name), err, backupDir,
			)
			return
		}
	}

	backupEntries, err := os.ReadDir(backupDir)
	if err != nil {
		log.Printf(
			"skills/snapshot: CRITICAL: rollback could not list backup %q: %v (manual intervention required)",
			backupDir, err,
		)
		return
	}
	for _, entry := range backupEntries {
		name := entry.Name()
		if err := os.Rename(filepath.Join(backupDir, name), filepath.Join(codebaseDir, name)); err != nil {
			log.Printf(
				"skills/snapshot: CRITICAL: rollback could not move %q back into %q: %v "+
					"(manual recovery: move the remaining contents of %q back up one level)",
				name, codebaseDir, err, backupDir,
			)
			return
		}
	}

	if err := os.Remove(backupDir); err != nil {
		// Non-critical: the tree is fully restored; only the empty backup shell
		// remains. Logged so the operator knows to sweep it.
		log.Printf("skills/snapshot: rollback complete but could not remove empty backup %q: %v", backupDir, err)
		return
	}
	log.Printf(
		"skills/snapshot: restore failed; original contents rolled back from %q into %q",
		backupDir, codebaseDir,
	)
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
// relative to the codebase root (-C sourceDir .), so destDir must be the
// codebase directory itself.
//
// Example equivalent shell-free invocation:
//
//	tar -xzf <archivePath> -C <codebaseDir>
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

	reclaimedLabel := "(none)"
	if len(result.ReclaimedAssets) > 0 {
		reclaimedLabel = strings.Join(result.ReclaimedAssets, ", ")
	}

	report := fmt.Sprintf(
		"status: ok\n"+
			"restored_from: %s\n"+
			"codebase_dir: %s\n"+
			"previous_contents_backup: %s\n"+
			"snapshots_store_preserved: %s\n"+
			"reclaimed_excluded_assets: %s\n"+
			"restored_at_utc: %s\n"+
			"note: prune old %s* backup directories manually once the restore is verified",
		result.ArchivePath,
		result.CodebaseDir,
		result.BackupDir,
		defaultDestDir,
		reclaimedLabel,
		time.Now().UTC().Format(time.RFC3339),
		backupNamePrefix,
	)

	if len(result.ReclaimWarnings) > 0 {
		report += "\n\nWARNINGS (restore succeeded; the entries below remain inside the backup " +
			"directory and must be moved back manually):\n  - " +
			strings.Join(result.ReclaimWarnings, "\n  - ")
	}

	return textResult(report), nil, nil
}
