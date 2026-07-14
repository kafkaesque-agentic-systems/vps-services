package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateComposeProjectDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}
	if err := validateComposeProjectDir(dir); err != nil {
		t.Errorf("validateComposeProjectDir: %v", err)
	}
}

func TestSystemUpAtRequiresEnvirons(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	_, err := systemUpAt(context.Background(), dir, filepath.Join(dir, ".environs"), false)
	if err == nil {
		t.Fatal("systemUpAt without .environs: want error")
	}
	if !strings.Contains(err.Error(), "environs") {
		t.Errorf("error = %q, want environs mention", err)
	}
}

func TestTruncateOutput(t *testing.T) {
	long := strings.Repeat("x", maxCommandOutput+100)
	out := truncateOutput(long)
	if len(out) <= maxCommandOutput+50 {
		// includes truncation suffix
		if !strings.Contains(out, "truncated") {
			t.Error("truncated output missing suffix")
		}
	}
}
