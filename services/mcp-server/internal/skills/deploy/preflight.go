package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const preflightTimeout = 10 * time.Minute

// callRemoteSnapshotCreate connects to the production MCP server over HTTPS+SSE
// and invokes snapshot_create with no arguments.
//
// It returns the textual tool output on success. Any connection, protocol, or
// tool-level error aborts the deployment before rsync runs.
func callRemoteSnapshotCreate(ctx context.Context, cfg *deployConfig) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, preflightTimeout)
	defer cancel()

	httpClient := newBearerHTTPClient(cfg.token)
	transport := &mcp.SSEClientTransport{
		Endpoint:   cfg.mcpURL,
		HTTPClient: httpClient,
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "deploy-preflight",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return "", fmt.Errorf("connect to production MCP at %q: %w", cfg.mcpURL, err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "snapshot_create",
		Arguments: map[string]any{},
	})
	if err != nil {
		return "", fmt.Errorf("call remote snapshot_create: %w", err)
	}
	if result.IsError {
		return "", fmt.Errorf("remote snapshot_create returned error: %s", extractToolText(result))
	}

	text := strings.TrimSpace(extractToolText(result))
	if text == "" {
		return "", fmt.Errorf("remote snapshot_create returned empty output")
	}
	return text, nil
}

// extractToolText concatenates all TextContent blocks from a CallToolResult.
func extractToolText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
