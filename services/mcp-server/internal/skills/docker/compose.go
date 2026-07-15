package docker

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
)

const (
	// defaultComposeProjectDir is the VPS directory containing docker-compose.yml.
	// LAYOUT MIGRATION (2026-07-14): the codebase moved from
	// /opt/micro-services.d/services up to /opt/micro-services.d itself.
	defaultComposeProjectDir = "/opt/micro-services.d"

	// defaultEnvironsPath is co-located with the compose project on the VPS.
	defaultEnvironsPath = defaultComposeProjectDir + "/.environs"

	// maxCommandOutput caps captured stdout/stderr from docker compose commands.
	maxCommandOutput = 256 << 10 // 256 KiB
)

// allowedComposeServices lists valid docker-compose.yml service keys for log filtering.
var allowedComposeServices = map[string]struct{}{
	"reverse-proxy": {},
	"web":           {},
	"api":           {},
	"dbs":           {},
	"go-mcp":        {},
}

// runCompose executes `docker compose <args...>` with cmd.Dir set to projectDir.
// extraEnv entries are appended to os.Environ() (e.g. parsed .environs pairs).
func runCompose(ctx context.Context, projectDir string, extraEnv []string, args ...string) (string, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return "", fmt.Errorf("locate docker binary: %w", err)
	}

	fullArgs := append([]string{"compose"}, args...)
	cmd := exec.CommandContext(ctx, dockerPath, fullArgs...)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(), extraEnv...)

	output, err := cmd.CombinedOutput()
	text := truncateOutput(string(output))
	if err != nil {
		return text, fmt.Errorf("run docker compose %v in %q: %w (output: %s)", args, projectDir, err, strings.TrimSpace(text))
	}

	return text, nil
}

// truncateOutput limits command output size to avoid exhausting container memory.
func truncateOutput(s string) string {
	if len(s) <= maxCommandOutput {
		return s
	}
	return s[:maxCommandOutput] + "\n\n... output truncated ..."
}

// validateComposeService returns an error if service is not a known compose key.
//
// The valid-service list in the error message is SORTED so the self-healing
// text handed to the calling LLM is deterministic across invocations (map
// iteration order in Go is randomized; an unstable error string would violate
// the deterministic-output directive and needlessly perturb model behavior).
func validateComposeService(service string) error {
	if _, ok := allowedComposeServices[service]; !ok {
		names := slices.Sorted(maps.Keys(allowedComposeServices))
		return fmt.Errorf("unknown service %q; valid services: %s", service, strings.Join(names, ", "))
	}
	return nil
}
