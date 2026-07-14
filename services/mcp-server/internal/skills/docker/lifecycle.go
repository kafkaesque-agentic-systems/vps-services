package docker

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolNameDown    = "system_down"
	toolNameUp      = "system_up"
	toolNameLogs    = "system_logs"
	defaultLogsTail = 100

	composeDownTimeout = 5 * time.Minute
	composeUpTimeout   = 20 * time.Minute
	composeLogsTimeout = 60 * time.Second
)

// DownInput is the input schema for system_down (no parameters).
type DownInput struct{}

// UpInput is the input schema for system_up.
type UpInput struct {
	// Build when true appends --build to force image rebuild before starting.
	Build bool `json:"build,omitempty" jsonschema:"Optional. When true, runs docker compose up -d --build to rebuild images before starting the stack."`
}

// LogsInput is the input schema for system_logs.
type LogsInput struct {
	// Service optionally limits logs to one compose service key (e.g. api, web, dbs).
	Service string `json:"service,omitempty" jsonschema:"Optional compose service name. Valid values: reverse-proxy, web, api, dbs, go-mcp. Omit to fetch logs for all services."`

	// Tail is the number of log lines from the end of each container (--tail). Defaults to 100.
	Tail int `json:"tail,omitempty" jsonschema:"Optional number of log lines to return from the end of each container log. Defaults to 100. Must not use follow/stream mode."`
}

// Register attaches every tool owned by the docker skill to the provided MCP server.
func Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameDown,
		Description: "Gracefully stops and removes all containers and project networks for the " +
			"micro-services stack via `docker compose down` in " + defaultComposeProjectDir + ". " +
			"Does NOT remove Docker volumes — persistent data (MongoDB quotes-api volume) is preserved. " +
			"Takes no arguments. Warning: this stops the go-mcp container itself, which kills the compose " +
			"client mid-teardown — shutdown may complete only PARTIALLY, this tool's success report may " +
			"never be delivered, and MCP will be unreachable afterward. An operator must verify and finish " +
			"the teardown on the VPS host (`docker compose ps`, then `docker compose down` / `up -d`).",
	}, handleSystemDown)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameUp,
		Description: "Starts the entire micro-services stack in detached mode via `docker compose up -d` " +
			"in " + defaultComposeProjectDir + ". Loads environment variables from " + defaultEnvironsPath +
			" before executing. Optional build=true adds --build to rebuild images.",
	}, handleSystemUp)

	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameLogs,
		Description: "Returns a static snapshot of container logs from " + defaultComposeProjectDir +
			" via `docker compose logs --tail N`. Optional service filters to one compose service. " +
			"Does not stream or follow logs. Default tail is 100 lines.",
	}, handleSystemLogs)

	log.Printf("skills/docker: registered tools %q, %q, %q", toolNameDown, toolNameUp, toolNameLogs)
}

// SystemDown stops the compose stack without removing volumes.
//
// Executed command (strictly no -v / --volumes):
//
//	docker compose down
func SystemDown(ctx context.Context) (string, error) {
	return systemDownAt(ctx, defaultComposeProjectDir)
}

func systemDownAt(ctx context.Context, projectDir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, composeDownTimeout)
	defer cancel()

	if err := validateComposeProjectDir(projectDir); err != nil {
		return "", err
	}

	return runCompose(ctx, projectDir, nil, composeDownArgs()...)
}

// composeDownArgs returns the exact, fixed argument slice for system_down.
//
// SAFETY INVARIANT (non-negotiable): this slice must NEVER contain -v,
// --volumes, --rmi, or any other flag that destroys persistent state. The
// external quotes-api MongoDB volume and all named/bind mounts must survive a
// stack shutdown to prevent data loss. The invariant is enforced by the
// regression test TestComposeDownArgsNeverRemoveVolumes, which inspects THIS
// production function — do not construct down arguments anywhere else.
func composeDownArgs() []string {
	return []string{"down"}
}

// SystemUp starts the compose stack, loading .environs into the command environment.
//
// Executed commands:
//
//	docker compose up -d
//	docker compose up -d --build   (when build=true)
func SystemUp(ctx context.Context, build bool) (string, error) {
	return systemUpAt(ctx, defaultComposeProjectDir, defaultEnvironsPath, build)
}

func systemUpAt(ctx context.Context, projectDir, environsPath string, build bool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, composeUpTimeout)
	defer cancel()

	if err := validateComposeProjectDir(projectDir); err != nil {
		return "", err
	}

	envPairs, err := ParseEnvirons(environsPath)
	if err != nil {
		return "", fmt.Errorf("load environment from %q: %w", environsPath, err)
	}

	args := []string{"up", "-d"}
	if build {
		args = append(args, "--build")
	}

	return runCompose(ctx, projectDir, envPairs, args...)
}

// SystemLogs returns a static log snapshot (never uses -f / --follow).
//
// Executed commands:
//
//	docker compose logs --tail <N>
//	docker compose logs --tail <N> <service>
func SystemLogs(ctx context.Context, service string, tail int) (string, error) {
	return systemLogsAt(ctx, defaultComposeProjectDir, service, tail)
}

func systemLogsAt(ctx context.Context, projectDir, service string, tail int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, composeLogsTimeout)
	defer cancel()

	if err := validateComposeProjectDir(projectDir); err != nil {
		return "", err
	}

	if tail <= 0 {
		tail = defaultLogsTail
	}

	service = strings.TrimSpace(service)
	if service != "" {
		if err := validateComposeService(service); err != nil {
			return "", err
		}
	}

	// Static snapshot only — never append -f or --follow (would block MCP handlers).
	args := []string{"logs", "--tail", strconv.Itoa(tail)}
	if service != "" {
		args = append(args, service)
	}

	return runCompose(ctx, projectDir, nil, args...)
}

func validateComposeProjectDir(projectDir string) error {
	info, err := os.Stat(projectDir)
	if err != nil {
		return fmt.Errorf("compose project directory %q: %w", projectDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("compose project path %q is not a directory", projectDir)
	}

	composeFile := projectDir + "/docker-compose.yml"
	if _, err := os.Stat(composeFile); err != nil {
		return fmt.Errorf("compose file %q: %w", composeFile, err)
	}

	return nil
}

func handleSystemDown(ctx context.Context, _ *mcp.CallToolRequest, _ DownInput) (*mcp.CallToolResult, any, error) {
	output, err := SystemDown(ctx)
	if err != nil {
		return errorResult("Failed to bring down micro-services stack: %v", err), nil, nil
	}

	report := fmt.Sprintf(
		"status: ok\ncommand: docker compose down\nproject_dir: %s\nvolumes_preserved: true\noutput:\n%s",
		defaultComposeProjectDir,
		strings.TrimSpace(output),
	)
	return textResult(report), nil, nil
}

func handleSystemUp(ctx context.Context, _ *mcp.CallToolRequest, in UpInput) (*mcp.CallToolResult, any, error) {
	output, err := SystemUp(ctx, in.Build)
	if err != nil {
		return errorResult("Failed to start micro-services stack: %v", err), nil, nil
	}

	cmd := "docker compose up -d"
	if in.Build {
		cmd = "docker compose up -d --build"
	}

	report := fmt.Sprintf(
		"status: ok\ncommand: %s\nproject_dir: %s\nenvirons: %s\noutput:\n%s",
		cmd,
		defaultComposeProjectDir,
		defaultEnvironsPath,
		strings.TrimSpace(output),
	)
	return textResult(report), nil, nil
}

func handleSystemLogs(ctx context.Context, _ *mcp.CallToolRequest, in LogsInput) (*mcp.CallToolResult, any, error) {
	output, err := SystemLogs(ctx, in.Service, in.Tail)
	if err != nil {
		return errorResult("Failed to retrieve container logs: %v", err), nil, nil
	}

	tail := in.Tail
	if tail <= 0 {
		tail = defaultLogsTail
	}

	serviceLabel := "all"
	serviceArg := ""
	if s := strings.TrimSpace(in.Service); s != "" {
		serviceLabel = s
		serviceArg = " " + s
	}

	report := fmt.Sprintf(
		"status: ok\ncommand: docker compose logs --tail %d%s\nproject_dir: %s\nservice: %s\noutput:\n%s",
		tail,
		serviceArg,
		defaultComposeProjectDir,
		serviceLabel,
		strings.TrimSpace(output),
	)
	return textResult(report), nil, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func errorResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
	}
}
