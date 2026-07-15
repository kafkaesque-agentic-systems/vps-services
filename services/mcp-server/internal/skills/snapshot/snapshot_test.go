package snapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func firstText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	if res == nil {
		t.Fatal("tool result is nil")
	}
	if len(res.Content) == 0 {
		t.Fatal("tool result has no content blocks")
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("first content block is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

func TestBuildTarArgs(t *testing.T) {
	args := buildTarArgs("/tmp/out.tar.gz", "/opt/source")

	want := []string{
		"-czf", "/tmp/out.tar.gz",
		"--exclude=image",
		"--exclude=vol",
		"--exclude=snapshots", // prevents recursive self-archiving (nested store)
		"--exclude=.bak-*",    // pre-restore backups must not snowball into archives
		"-C", "/opt/source",
		".",
	}

	if len(args) != len(want) {
		t.Fatalf("len(args) = %d, want %d; args=%v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestCreateSnapshotAt(t *testing.T) {
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar binary not available in PATH")
	}

	sourceDir := t.TempDir()
	destDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.Mkdir(filepath.Join(sourceDir, "image"), 0o755); err != nil {
		t.Fatalf("mkdir image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "image", "skip.png"), []byte("png"), 0o644); err != nil {
		t.Fatalf("write image/skip.png: %v", err)
	}
	if err := os.Mkdir(filepath.Join(sourceDir, "vol"), 0o755); err != nil {
		t.Fatalf("mkdir vol: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "vol", "data.bin"), []byte("bin"), 0o644); err != nil {
		t.Fatalf("write vol/data.bin: %v", err)
	}
	// Post-migration layout: the snapshot store lives inside the source tree
	// and must NEVER be swallowed into an archive (recursive growth).
	if err := os.Mkdir(filepath.Join(sourceDir, "snapshots"), 0o755); err != nil {
		t.Fatalf("mkdir snapshots: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "snapshots", "old.tar.gz"), []byte("gz"), 0o644); err != nil {
		t.Fatalf("write snapshots/old.tar.gz: %v", err)
	}

	archivePath, err := createSnapshotAt(context.Background(), sourceDir, destDir)
	if err != nil {
		t.Fatalf("createSnapshotAt returned error: %v", err)
	}

	if !strings.HasPrefix(filepath.Base(archivePath), snapshotFilenamePrefix) {
		t.Errorf("archive basename = %q, want prefix %q", filepath.Base(archivePath), snapshotFilenamePrefix)
	}
	if !strings.HasSuffix(archivePath, ".tar.gz") {
		t.Errorf("archive path = %q, want .tar.gz suffix", archivePath)
	}

	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("stat archive: %v", err)
	}

	listCmd := exec.Command("tar", "-tzf", archivePath)
	output, err := listCmd.Output()
	if err != nil {
		t.Fatalf("list archive contents: %v", err)
	}

	listing := string(output)
	if !strings.Contains(listing, "./main.go") && !strings.Contains(listing, "main.go") {
		t.Errorf("archive listing missing main.go; listing=%q", listing)
	}
	if strings.Contains(listing, "image/") || strings.Contains(listing, "vol/") || strings.Contains(listing, "snapshots/") {
		t.Errorf("archive listing contains excluded directory; listing=%q", listing)
	}
}

func TestCreateSnapshotAtMissingSource(t *testing.T) {
	destDir := t.TempDir()
	missingSource := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := createSnapshotAt(context.Background(), missingSource, destDir)
	if err == nil {
		t.Fatal("createSnapshotAt with missing source: want error, got nil")
	}
	if !strings.Contains(err.Error(), "source directory") {
		t.Errorf("error = %q, want it to mention source directory", err)
	}
}

func TestHandleSnapshotCreateSuccess(t *testing.T) {
	// OPT-IN END-TO-END TEST — never runs implicitly.
	//
	// This handler test archives the REAL default production tree
	// (/opt/micro-services.d). On a developer machine that path is absent; on
	// the VPS it exists, and an implicit run would either fail on permissions
	// (the host shell user is not the container's mcp user) or — far worse —
	// silently write a real archive into the production snapshots store as a
	// unit-test side effect. Side-effectful operations against production
	// paths require a deliberate operator opt-in.
	//
	// The success PATH itself is fully covered without side effects by
	// TestCreateSnapshotAt and the restore round-trip tests (temp dirs).
	if os.Getenv("MCP_SNAPSHOT_E2E") != "1" {
		t.Skip("set MCP_SNAPSHOT_E2E=1 to run the end-to-end snapshot handler test (writes a REAL archive to the production snapshots store)")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar binary not available in PATH")
	}
	if _, err := os.Stat(defaultSourceDir); err != nil {
		t.Skip("default source directory not present; skipping handler success test")
	}

	res, _, err := handleSnapshotCreate(context.Background(), nil, SnapshotInput{})
	if err != nil {
		t.Fatalf("handleSnapshotCreate returned Go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError = true, want false; text=%q", firstText(t, res))
	}

	text := firstText(t, res)
	if !strings.Contains(text, "status: ok") {
		t.Errorf("success text = %q, want status: ok", text)
	}
	if !strings.Contains(text, "archive:") {
		t.Errorf("success text = %q, want archive path", text)
	}
	if !strings.Contains(text, "excluded: image, vol, snapshots, .bak-*") {
		t.Errorf("success text = %q, want full exclusion list incl. snapshots and .bak-*", text)
	}
}

func TestHandleSnapshotCreateMissingSource(t *testing.T) {
	// Force failure by relying on default VPS paths that do not exist in CI/local.
	if _, err := os.Stat(defaultSourceDir); err == nil {
		t.Skip("default source directory exists; skipping missing-source handler test")
	}

	res, goErr, err := handleSnapshotCreate(context.Background(), nil, SnapshotInput{})
	if err != nil {
		t.Fatalf("handleSnapshotCreate returned unexpected error from handler signature: %v", err)
	}
	if goErr != nil {
		t.Fatalf("handleSnapshotCreate returned Go error %v, want nil (errors via IsError)", goErr)
	}
	if !res.IsError {
		t.Fatalf("IsError = false, want true; text=%q", firstText(t, res))
	}

	text := firstText(t, res)
	if !strings.Contains(text, "Failed to create codebase snapshot") {
		t.Errorf("error text = %q, want failure message prefix", text)
	}
}
