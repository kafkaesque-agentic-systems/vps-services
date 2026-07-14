package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnvirons(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".environs")
	content := `# comment
export MAILSERVER=user@example.com
export MONGO_DATABASE=qdb
export AUTHORIZED='token-with-quotes'
export EMPTY=

`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write environs: %v", err)
	}

	pairs, err := ParseEnvirons(path)
	if err != nil {
		t.Fatalf("ParseEnvirons: %v", err)
	}

	want := map[string]string{
		"MAILSERVER":     "user@example.com",
		"MONGO_DATABASE": "qdb",
		"AUTHORIZED":     "token-with-quotes",
		"EMPTY":          "",
	}

	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		key, val, ok := strings.Cut(p, "=")
		if !ok {
			t.Fatalf("invalid pair %q", p)
		}
		got[key] = val
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseEnvironsMissingFile(t *testing.T) {
	_, err := ParseEnvirons(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("ParseEnvirons missing file: want error")
	}
}

func TestValidateComposeService(t *testing.T) {
	if err := validateComposeService("api"); err != nil {
		t.Errorf("validateComposeService(api): %v", err)
	}
	if err := validateComposeService("nginx"); err == nil {
		t.Error("validateComposeService(nginx): want error")
	}
}

func TestSystemLogsTailDefault(t *testing.T) {
	if defaultLogsTail != 100 {
		t.Fatalf("defaultLogsTail = %d, want 100", defaultLogsTail)
	}
}

func TestHandleSystemLogsInvalidService(t *testing.T) {
	res, _, err := handleSystemLogs(t.Context(), nil, LogsInput{Service: "nginx", Tail: 10})
	if err != nil {
		t.Fatalf("handleSystemLogs returned Go error: %v", err)
	}
	if !res.IsError {
		t.Fatal("IsError = false, want true for invalid service")
	}
}

func TestHandleSystemDownProjectMissing(t *testing.T) {
	res, _, err := handleSystemDown(t.Context(), nil, DownInput{})
	if err != nil {
		t.Fatalf("handleSystemDown returned Go error: %v", err)
	}
	if !res.IsError {
		t.Skip("default compose project exists locally; skipping missing-project test")
	}
}

// TestComposeDownArgsNeverRemoveVolumes locks in the single most important
// safety invariant of this skill: system_down must never destroy persistent
// state. Unlike a copy of the expected slice, this test inspects the REAL
// production function (composeDownArgs) that systemDownAt executes, so any
// future change that sneaks a destructive flag into the down path fails CI.
func TestComposeDownArgsNeverRemoveVolumes(t *testing.T) {
	args := composeDownArgs()

	if len(args) == 0 || args[0] != "down" {
		t.Fatalf("composeDownArgs() = %v, want first argument \"down\"", args)
	}

	forbidden := []string{"-v", "--volumes", "--rmi"}
	for _, arg := range args {
		for _, bad := range forbidden {
			if arg == bad || strings.HasPrefix(arg, bad+"=") {
				t.Fatalf("composeDownArgs() contains forbidden destructive flag %q: %v", bad, args)
			}
		}
	}
}

// TestValidateComposeServiceErrorIsDeterministic verifies the self-healing
// error text is stable across calls (the valid-service list must be sorted,
// not emitted in random map-iteration order) and enumerates every allowed
// service so the calling LLM can correct its next invocation.
func TestValidateComposeServiceErrorIsDeterministic(t *testing.T) {
	err1 := validateComposeService("nope")
	err2 := validateComposeService("nope")
	if err1 == nil || err2 == nil {
		t.Fatal("validateComposeService(nope): want error")
	}
	if err1.Error() != err2.Error() {
		t.Errorf("error text is unstable across calls:\n%q\n%q", err1, err2)
	}
	if want := "api, dbs, go-mcp, reverse-proxy, web"; !strings.Contains(err1.Error(), want) {
		t.Errorf("error = %q, want sorted service list %q", err1, want)
	}
}
