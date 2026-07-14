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
	result, err := restoreSnapshotAt(context.Background(), filename, snapshotsDir, servicesDir, parentDir)
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
		parentDir,
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
		parentDir,
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
