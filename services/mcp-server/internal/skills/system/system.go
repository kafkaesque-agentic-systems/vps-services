package system

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"mcp-server/internal/skills/quote"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool name constants. Tool names are part of the server's public contract with
// clients (an LLM invokes a tool by this exact string), so they are defined once
// here to keep the registration and any future references perfectly in sync.
//
// Naming convention: "<domain>_<action>". Prefixing every tool in this package
// with "system_" namespaces the domain, which keeps tool names unambiguous once
// many skills are registered on the same server.
const (
	// toolNameHealth is the operational health / uptime probe.
	toolNameHealth = "system_health"

	// toolNameTime reports the current server time, optionally in a caller-
	// specified IANA timezone.
	toolNameTime = "system_time"

	// toolNameQuote fetches a random formatted quote.
	toolNameQuote = "quote_random"
)

// startTime records the moment this package (and therefore the process) was
// initialized. It is captured exactly once at load time and never mutated, so it
// is safe to read concurrently from many tool invocations without locking.
//
// It is the reference point for the uptime reported by the system_health tool.
var startTime = time.Now()

// HealthInput is the input schema for the system_health tool.
//
// The tool intentionally takes NO parameters: a health probe should be callable
// with zero configuration. Declaring it as an empty struct causes the SDK to
// generate a JSON schema describing an object with no properties, which
// communicates to clients that no arguments are expected.
type HealthInput struct{}

// TimeInput is the input schema for the system_time tool.
//
// The `jsonschema` struct tag supplies the human/LLM-facing description that the
// SDK embeds in the generated JSON schema. A precise description is a first-class
// part of the self-healing story: the better the schema documents the field, the
// more likely the calling model is to get the argument right on the first try.
type TimeInput struct {
	// Timezone is an optional IANA timezone name (e.g. "UTC",
	// "America/New_York"). When empty, the tool defaults to UTC. Semantic
	// validity (whether the string is a real tz-database entry) cannot be
	// expressed in JSON Schema and is therefore validated inside the handler,
	// which returns a self-healing error on failure.
	Timezone string `json:"timezone,omitempty" jsonschema:"Optional IANA timezone name such as 'UTC', 'America/New_York', 'Europe/London', or 'Asia/Tokyo'. If omitted, UTC is used."`
}

// QuoteInput is the input schema for the quote_random tool.
type QuoteInput struct {
	// Takes no parameters.
}

// Register attaches every tool owned by the system skill to the provided MCP
// server.
//
// # Contract
//
// This is the single exported entry point of the package — the composition root
// (cmd/server) calls it via registerCustomSkills. Keeping registration behind
// one function means the domain can grow (more tools, prompts, resources)
// without the caller ever needing to change: it always just calls Register.
//
// # Why mcp.AddTool (package function) and not server.AddTool (method)
//
// The generic package-level mcp.AddTool[In, Out] infers and installs the JSON
// input schema from the typed In struct via reflection. That automatic,
// type-derived schema is precisely the structural-validation layer our
// self-healing strategy relies on, so it is the correct registration primitive
// for typed tools.
func Register(server *mcp.Server) {
	// system_health: a zero-argument operational probe.
	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameHealth,
		Description: "Reports the operational health of the MCP server, including its " +
			"liveness status, process uptime, Go runtime version, active goroutine " +
			"count, and current UTC time. Takes no arguments. Use this to verify the " +
			"server is reachable and healthy.",
	}, handleHealth)

	// system_time: current time, with optional timezone and rich validation.
	mcp.AddTool(server, &mcp.Tool{
		Name: toolNameTime,
		Description: "Returns the current server time. Optionally accepts a 'timezone' " +
			"argument (an IANA timezone name such as 'America/New_York'); if omitted, " +
			"the time is returned in UTC. Returns a descriptive, correctable error if " +
			"the provided timezone is not a valid IANA name.",
	}, handleTime)

	// quote_random: fetches a random formatted quote from the ThirdEye API.
	mcp.AddTool(server, &mcp.Tool{
		Name:        toolNameQuote,
		Description: "Fetches a random quote from the ThirdEye API. Returns the quote formatted cleanly as 'quote -attribution'. Takes no arguments.",
	}, handleQuote)

	log.Printf("skills/system: registered tools %q, %q, %q", toolNameHealth, toolNameTime, toolNameQuote)
}

// handleHealth implements the system_health tool.
//
// It never fails on input (there is none) and never panics; it simply snapshots
// a few cheap runtime metrics and returns them as human-readable text. The
// snapshot is taken with standard library calls that are safe for concurrent
// use, so no synchronization is required.
//
// The handler signature is mandated by the SDK's ToolHandlerFor[In, Out] type:
//
//	func(ctx, *CallToolRequest, In) (*CallToolResult, Out, error)
//
// We use `any` for the Out type and return nil for it, meaning this tool exposes
// no structured-output schema — all information is conveyed in the text content,
// which is what an LLM consumes.
func handleHealth(_ context.Context, _ *mcp.CallToolRequest, _ HealthInput) (*mcp.CallToolResult, any, error) {
	uptime := time.Since(startTime)

	// Build a clear, self-describing report. Each metric is labeled so the model
	// (or a human reading a transcript) can interpret it without external
	// context.
	report := fmt.Sprintf(
		"status: ok\n"+
			"uptime: %s (%.0f seconds)\n"+
			"go_version: %s\n"+
			"num_goroutine: %d\n"+
			"server_time_utc: %s",
		uptime.Round(time.Second),
		uptime.Seconds(),
		runtime.Version(),
		runtime.NumGoroutine(),
		time.Now().UTC().Format(time.RFC3339),
	)

	return textResult(report), nil, nil
}

// handleTime implements the system_time tool, demonstrating the full
// self-healing validation pattern.
//
// Flow:
//   - An absent/empty timezone is valid and defaults to UTC (documented in the
//     schema), so the common case needs no special handling.
//   - A non-empty timezone is validated with time.LoadLocation. Because "is this
//     a real IANA timezone?" cannot be encoded in JSON Schema, this semantic
//     check lives here. On failure we return a tool-level error (IsError: true)
//     whose text names the offending value AND lists concrete valid examples,
//     so the LLM can immediately correct its next invocation. Crucially we
//     return a nil Go error, ensuring the guidance reaches the model as tool
//     output rather than being swallowed as a protocol-level failure.
func handleTime(_ context.Context, _ *mcp.CallToolRequest, in TimeInput) (*mcp.CallToolResult, any, error) {
	loc := time.UTC

	if tz := strings.TrimSpace(in.Timezone); tz != "" {
		parsed, err := time.LoadLocation(tz)
		if err != nil {
			// Self-healing error: specific, actionable, and correctable.
			return errorResult(
				"Invalid 'timezone' value %q: %v. Provide a valid IANA timezone "+
					"name, for example: \"UTC\", \"America/New_York\", \"Europe/London\", "+
					"or \"Asia/Tokyo\". Alternatively, omit the 'timezone' argument "+
					"entirely to receive the time in UTC.",
				tz, err,
			), nil, nil
		}
		loc = parsed
	}

	now := time.Now().In(loc)
	text := fmt.Sprintf(
		"Current server time in %s:\n%s",
		loc.String(),
		now.Format(time.RFC3339),
	)

	return textResult(text), nil, nil
}

// handleQuote implements the quote_random tool.
// It calls the FetchRandomQuote business logic from the sibling package.
func handleQuote(ctx context.Context, _ *mcp.CallToolRequest, _ QuoteInput) (*mcp.CallToolResult, any, error) {
	formattedQuote, err := quote.FetchRandomQuote(ctx)
	if err != nil {
		// Self-healing error reporting patterns used in your framework
		return errorResult("Failed to fetch random quote from API: %v", err), nil, nil
	}

	return textResult(formattedQuote), nil, nil
}

// textResult builds a successful tool result carrying a single block of text.
//
// Centralizing result construction keeps every handler terse and guarantees a
// consistent result shape across the whole skill. The SDK's *mcp.TextContent is
// used via pointer because that is the form that satisfies the mcp.Content
// interface.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// errorResult builds a tool result flagged as an error (IsError: true) whose
// text is a descriptive, self-healing message intended for the calling LLM.
//
// This is the cornerstone of directive 2.4: rather than returning a Go error
// (which the SDK would surface as a protocol failure the model cannot easily
// reason about), a handler calls errorResult to hand the model a clear
// explanation of what went wrong and how to fix its next call. The signature
// mirrors fmt.Sprintf so callers can format context directly into the message.
func errorResult(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf(format, args...)},
		},
	}
}
