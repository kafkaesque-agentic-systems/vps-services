package snapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSnapshotFilename(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "snapshot-2026-07-12_12-00-00.tar.gz", wantErr: false},
		{name: "empty", input: "", wantErr: true},
		{name: "path separator", input: "../snapshot-2026-07-12_12-00-00.tar.gz", wantErr: true},
		{name: "wrong prefix", input: "backup-2026-07-12_12-00-00.tar.gz", wantErr: true},
		{name: "wrong suffix", input: "snapshot-2026-07-12_12-00-00.tar", wantErr: true},
		{name: "dotdot basename", input: "..tar.gz", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSnapshotFilename(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSnapshotFilename(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestBuildExtractTarArgs(t *testing.T) {
	args := buildExtractTarArgs("/snapshots/snap.tar.gz", "/opt/services")

	want := []string{"-xzf", "/snapshots/snap.tar.gz", "-C", "/opt/services"}
	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d; args=%v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestRestoreSnapshotRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar binary not available in PATH")
	}

	parentDir := t.TempDir()
	servicesDir := filepath.Join(parentDir, "services")
	snapshotsDir := filepath.Join(parentDir, "snapshots")

	if err := os.Mkdir(servicesDir, 0o755); err != nil {
		t.Fatalf("mkdir services: %v", err)
	}
	original := []byte("version-one\n")
	if err := os.WriteFile(filepath.Join(servicesDir, "main.go"), original, 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	archivePath, err := createSnapshotAt(context.Background(), servicesDir, snapshotsDir)
	if err != nil {
		t.Fatalf("createSnapshotAt: %v", err)
	}

	modified := []byte("version-two\n")
	if err := os.WriteFile(filepath.Join(servicesDir, "main.go"), modified, 0o644); err != nil {
		t.Fatalf("write modified main.go: %v", err)
	}

	filename := filepath.Base(archivePath)
	result, err := restoreSnapshotAt(context.Background(), filename, snapshotsDir, servicesDir)
	if err != nil {
		t.Fatalf("restoreSnapshotAt: %v", err)
	}

	restored, err := os.ReadFile(filepath.Join(servicesDir, "main.go"))
	if err != nil {
		t.Fatalf("read restored main.go: %v", err)
	}
	if string(restored) != string(original) {
		t.Errorf("restored content = %q, want %q", restored, original)
	}

	if _, err := os.Stat(result.BackupDir); err != nil {
		t.Fatalf("backup dir missing at %q: %v", result.BackupDir, err)
	}
	backupContent, err := os.ReadFile(filepath.Join(result.BackupDir, "main.go"))
	if err != nil {
		t.Fatalf("read backup main.go: %v", err)
	}
	if string(backupContent) != string(modified) {
		t.Errorf("backup content = %q, want modified %q", backupContent, modified)
	}
}

func TestRestoreSnapshotAtMissingArchive(t *testing.T) {
	parentDir := t.TempDir()
	servicesDir := filepath.Join(parentDir, "services")
	snapshotsDir := filepath.Join(parentDir, "snapshots")

	if err := os.MkdirAll(servicesDir, 0o755); err != nil {
		t.Fatalf("mkdir services: %v", err)
	}
	if err := os.MkdirAll(snapshotsDir, 0o755); err != nil {
		t.Fatalf("mkdir snapshots: %v", err)
	}

	_, err := restoreSnapshotAt(
		context.Background(),
		"snapshot-2026-07-12_12-00-00.tar.gz",
		snapshotsDir,
		servicesDir,
	)
	if err == nil {
		t.Fatal("restoreSnapshotAt with missing archive: want error, got nil")
	}
	if !strings.Contains(err.Error(), "snapshot archive") {
		t.Errorf("error = %q, want snapshot archive mention", err)
	}
}

func TestRestoreSnapshotAtRollbackOnBadArchive(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar binary not available in PATH")
	}

	parentDir := t.TempDir()
	servicesDir := filepath.Join(parentDir, "services")
	snapshotsDir := filepath.Join(parentDir, "snapshots")

	if err := os.MkdirAll(servicesDir, 0o755); err != nil {
		t.Fatalf("mkdir services: %v", err)
	}
	if err := os.MkdirAll(snapshotsDir, 0o755); err != nil {
		t.Fatalf("mkdir snapshots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(servicesDir, "keep.txt"), []byte("stay"), 0o644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}

	badArchive := filepath.Join(snapshotsDir, "snapshot-2026-07-12_12-00-00.tar.gz")
	if err := os.WriteFile(badArchive, []byte("not-a-tarball"), 0o644); err != nil {
		t.Fatalf("write bad archive: %v", err)
	}

	_, err := restoreSnapshotAt(
		context.Background(),
		filepath.Base(badArchive),
		snapshotsDir,
		servicesDir,
	)
	if err == nil {
		t.Fatal("restoreSnapshotAt with corrupt archive: want error, got nil")
	}

	content, err := os.ReadFile(filepath.Join(servicesDir, "keep.txt"))
	if err != nil {
		t.Fatalf("active services not rolled back; read keep.txt: %v", err)
	}
	if string(content) != "stay" {
		t.Errorf("active services content = %q, want original after rollback", content)
	}
}

// TestRestoreSnapshotNestedStoreRoundTrip exercises the PRODUCTION layout after
// the 2026-07-14 migration: the snapshots store is a child of the codebase
// tree, and the tree root itself is (in production) an unrenamable bind-mount
// point. The contents-swap restore must (a) succeed with the store untouched
// at its original path, (b) place the pre-restore contents in a .bak-<ts>
// backup INSIDE the tree, and (c) not extract a nested snapshots directory
// (archives exclude it).
func TestRestoreSnapshotNestedStoreRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar binary not available in PATH")
	}

	parentDir := t.TempDir()
	codebaseDir := filepath.Join(parentDir, "micro-services.d")
	snapshotsDir := filepath.Join(codebaseDir, "snapshots") // NESTED

	if err := os.MkdirAll(codebaseDir, 0o755); err != nil {
		t.Fatalf("mkdir codebase: %v", err)
	}
	original := []byte("version-one\n")
	if err := os.WriteFile(filepath.Join(codebaseDir, "docker-compose.yml"), original, 0o644); err != nil {
		t.Fatalf("write docker-compose.yml: %v", err)
	}
	// vol/ is excluded from archives; the restore must preserve it IN PLACE
	// (production carries vol/quotesdb.volume — losing it would drop
	// persistent runtime data from the active tree).
	if err := os.Mkdir(filepath.Join(codebaseDir, "vol"), 0o755); err != nil {
		t.Fatalf("mkdir vol: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codebaseDir, "vol", "quotesdb.volume"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write vol/quotesdb.volume: %v", err)
	}
	// prx/image is the NESTED excluded case (production: proxy static assets).
	// Its parent prx/ participates in the swap, so image/ must come back via
	// the post-extraction reclaim pass.
	if err := os.MkdirAll(filepath.Join(codebaseDir, "prx", "image"), 0o755); err != nil {
		t.Fatalf("mkdir prx/image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codebaseDir, "prx", "nginx.conf"), []byte("conf"), 0o644); err != nil {
		t.Fatalf("write prx/nginx.conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codebaseDir, "prx", "image", "logo.png"), []byte("png"), 0o644); err != nil {
		t.Fatalf("write prx/image/logo.png: %v", err)
	}

	archivePath, err := createSnapshotAt(context.Background(), codebaseDir, snapshotsDir)
	if err != nil {
		t.Fatalf("createSnapshotAt: %v", err)
	}

	modified := []byte("version-two\n")
	if err := os.WriteFile(filepath.Join(codebaseDir, "docker-compose.yml"), modified, 0o644); err != nil {
		t.Fatalf("write modified docker-compose.yml: %v", err)
	}

	result, err := restoreSnapshotAt(context.Background(), filepath.Base(archivePath), snapshotsDir, codebaseDir)
	if err != nil {
		t.Fatalf("restoreSnapshotAt (nested store): %v", err)
	}

	// (a): the archive must still exist at its ORIGINAL path — the store is
	// never moved by the contents-swap.
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive missing from restored tree at %q: %v", archivePath, err)
	}

	// (b): the backup lives INSIDE the tree (mountpoint constraint) and is
	// named with the backup prefix.
	if filepath.Dir(result.BackupDir) != codebaseDir {
		t.Errorf("BackupDir = %q, want a child of %q", result.BackupDir, codebaseDir)
	}
	if !strings.HasPrefix(filepath.Base(result.BackupDir), backupNamePrefix) {
		t.Errorf("BackupDir basename = %q, want prefix %q", filepath.Base(result.BackupDir), backupNamePrefix)
	}

	restored, err := os.ReadFile(filepath.Join(codebaseDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restored) != string(original) {
		t.Errorf("restored content = %q, want %q", restored, original)
	}

	// The backup must hold the pre-restore (modified) contents WITHOUT the
	// snapshots store, which is preserved in place and never swapped.
	backupContent, err := os.ReadFile(filepath.Join(result.BackupDir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read backup file: %v", err)
	}
	if string(backupContent) != string(modified) {
		t.Errorf("backup content = %q, want modified %q", backupContent, modified)
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, "snapshots")); !os.IsNotExist(err) {
		t.Errorf("backup contains a snapshots store (stat err = %v); the store must stay in place", err)
	}

	// (c): no nested snapshots/snapshots artifact.
	if _, err := os.Stat(filepath.Join(snapshotsDir, "snapshots")); !os.IsNotExist(err) {
		t.Errorf("restored tree contains a nested snapshots/snapshots directory (stat err = %v)", err)
	}

	// (d): archive-excluded runtime assets are preserved IN PLACE — vol/ must
	// still be live in the restored tree, not stranded in the backup.
	if _, err := os.Stat(filepath.Join(codebaseDir, "vol", "quotesdb.volume")); err != nil {
		t.Errorf("vol/quotesdb.volume missing from restored tree (must be preserved in place): %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, "vol")); !os.IsNotExist(err) {
		t.Errorf("backup contains vol/ (stat err = %v); excluded assets must never be swapped out", err)
	}

	// (e): NESTED excluded assets are reclaimed after extraction — prx/image
	// swapped out with its parent, is absent from the archive, and must be
	// moved back into the restored tree.
	if _, err := os.Stat(filepath.Join(codebaseDir, "prx", "image", "logo.png")); err != nil {
		t.Errorf("prx/image/logo.png missing from restored tree (reclaim pass failed): %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.BackupDir, "prx", "image")); !os.IsNotExist(err) {
		t.Errorf("backup still contains prx/image (stat err = %v); it should have been reclaimed", err)
	}
	// The parent prx/ itself WAS swapped and restored from the archive.
	if _, err := os.Stat(filepath.Join(codebaseDir, "prx", "nginx.conf")); err != nil {
		t.Errorf("prx/nginx.conf missing from restored tree: %v", err)
	}
	if len(result.ReclaimedAssets) != 1 || result.ReclaimedAssets[0] != filepath.Join("prx", "image") {
		t.Errorf("ReclaimedAssets = %v, want [prx/image]", result.ReclaimedAssets)
	}
	if len(result.ReclaimWarnings) != 0 {
		t.Errorf("ReclaimWarnings = %v, want none", result.ReclaimWarnings)
	}
}

// TestRestoreSnapshotNestedStoreRollback verifies that a corrupt archive in the
// nested layout rolls back BOTH moves: the codebase returns, and the snapshots
// store (with the archive) returns into it.
func TestRestoreSnapshotNestedStoreRollback(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar binary not available in PATH")
	}

	parentDir := t.TempDir()
	codebaseDir := filepath.Join(parentDir, "micro-services.d")
	snapshotsDir := filepath.Join(codebaseDir, "snapshots")

	if err := os.MkdirAll(snapshotsDir, 0o755); err != nil {
		t.Fatalf("mkdir snapshots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codebaseDir, "keep.txt"), []byte("stay"), 0o644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}
	badArchive := filepath.Join(snapshotsDir, "snapshot-2026-07-14_12-00-00.tar.gz")
	if err := os.WriteFile(badArchive, []byte("not-a-tarball"), 0o644); err != nil {
		t.Fatalf("write bad archive: %v", err)
	}

	_, err := restoreSnapshotAt(context.Background(), filepath.Base(badArchive), snapshotsDir, codebaseDir)
	if err == nil {
		t.Fatal("restoreSnapshotAt with corrupt archive: want error, got nil")
	}

	content, err := os.ReadFile(filepath.Join(codebaseDir, "keep.txt"))
	if err != nil {
		t.Fatalf("codebase not rolled back; read keep.txt: %v", err)
	}
	if string(content) != "stay" {
		t.Errorf("codebase content = %q, want original after rollback", content)
	}
	if _, err := os.Stat(badArchive); err != nil {
		t.Errorf("snapshots store not rolled back into the codebase tree; stat archive: %v", err)
	}
}

// TestNestedSnapshotsTopComponent pins the nesting-detection matrix.
func TestNestedSnapshotsTopComponent(t *testing.T) {
	top, err := nestedSnapshotsTopComponent("/opt/micro-services.d", "/opt/micro-services.d/snapshots")
	if err != nil || top != "snapshots" {
		t.Errorf("nested case: top=%q err=%v, want snapshots/nil", top, err)
	}

	top, err = nestedSnapshotsTopComponent("/opt/micro-services.d", "/opt/micro-services.d/backups/snapshots")
	if err != nil || top != "backups" {
		t.Errorf("deep-nested case: top=%q err=%v, want backups/nil (first component shields the subtree)", top, err)
	}

	top, err = nestedSnapshotsTopComponent("/opt/micro-services.d/services", "/opt/micro-services.d/snapshots")
	if err != nil || top != "" {
		t.Errorf("sibling case: top=%q err=%v, want empty/nil", top, err)
	}

	if _, err := nestedSnapshotsTopComponent("/opt/x", "/opt/x"); err == nil {
		t.Error("identical paths: want error (store must not be the codebase itself)")
	}
}

func TestHandleSnapshotRestoreMissingFilename(t *testing.T) {
	res, _, err := handleSnapshotRestore(context.Background(), nil, RestoreInput{})
	if err != nil {
		t.Fatalf("handleSnapshotRestore returned Go error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("IsError = false, want true")
	}
	text := firstText(t, res)
	if !strings.Contains(text, "Missing required 'filename'") {
		t.Errorf("text = %q, want missing filename guidance", text)
	}
}
