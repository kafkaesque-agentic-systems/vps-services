package deploy

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const toolNamePush = "push_codebase"

// PushInput is the input schema for push_codebase.
//
// The tool intentionally takes NO parameters: deployment paths and exclusions
// are fixed operational constants resolved from environment variables.
type PushInput struct{}

// Register attaches every tool owned by the deploy skill to the provided MCP server.
func Register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: toolNamePush,
		Description: "Synchronizes the local micro-services codebase to the production VPS via rsync over SSH. " +
			"LOCAL-ONLY: must run on a locally started mcp-server with rsync, SSH access, and DEPLOY_SSH_TARGET set. " +
			"Pre-flight: calls remote production snapshot_create over HTTPS+SSE before syncing. " +
			"Uses rsync -az --delete -i with exclusions for .git/, node_modules/, .venv/, __pycache__/, .env, .environs, image/, vol/, snapshots/ (protects the VPS snapshot store from --delete), deploy_ledgers/. " +
			"Writes an itemized ledger to deploy_ledgers/deploy-YYYY-MM-DD_HH-MM-SS.log and returns it in the response. " +
			"Takes no arguments.",
	}, handlePushCodebase)

	log.Printf("skills/deploy: registered tool %q", toolNamePush)
}

// handlePushCodebase implements the push_codebase MCP tool lifecycle:
//  1. Resolve and validate local deployment configuration.
//  2. Pre-flight remote snapshot_create on production MCP.
//  3. Execute rsync and return the itemized ledger.
func handlePushCodebase(ctx context.Context, _ *mcp.CallToolRequest, _ PushInput) (*mcp.CallToolResult, any, error) {
	cfg, err := resolveDeployConfig()
	if err != nil {
		return errorResult("push_codebase configuration error: %v", err), nil, nil
	}

	snapshotReport, err := callRemoteSnapshotCreate(ctx, cfg)
	if err != nil {
		return errorResult("Pre-flight snapshot_create failed; deployment aborted: %v", err), nil, nil
	}

	syncResult, err := runRsync(ctx, cfg)
	if err != nil {
		return errorResult("rsync sync failed after successful pre-flight snapshot: %v", err), nil, nil
	}

	report := buildPushReport(cfg, snapshotReport, syncResult)
	return textResult(report), nil, nil
}

// buildPushReport assembles the tool response with snapshot summary, sync
// metadata, and the full itemized rsync ledger.
func buildPushReport(cfg *deployConfig, snapshotReport string, sync *rsyncResult) string {
	var b strings.Builder
	b.WriteString("status: ok\n")
	b.WriteString("phase: push_codebase\n\n")

	b.WriteString("--- pre_flight snapshot_create ---\n")
	b.WriteString(snapshotReport)
	b.WriteString("\n\n")

	b.WriteString("--- sync ---\n")
	b.WriteString(fmt.Sprintf("local_root: %s\n", cfg.localRoot))
	b.WriteString(fmt.Sprintf("remote_target: %s:%s\n", cfg.sshTarget, cfg.remotePath))
	b.WriteString(fmt.Sprintf("ledger_path: %s\n\n", sync.ledgerPath))

	b.WriteString("--- itemized ledger (rsync -i) ---\n")
	b.WriteString("Legend: >f+++++++++ new file; >f..T...... updated file; cd+++++++++ new dir; *deleting removed stale path\n\n")
	if sync.ledgerText != "" {
		b.WriteString(sync.ledgerText)
	} else if sync.commandOutput != "" {
		b.WriteString(sync.commandOutput)
	} else {
		b.WriteString("(no file changes recorded)")
	}
	b.WriteByte('\n')

	return b.String()
}

// textResult builds a successful tool result carrying a single block of text.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// errorResult builds a tool result flagged as an error (IsError: true) whose
// text is a descriptive, self-healing message intended for the calling LLM.
func errorResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
	}
}
