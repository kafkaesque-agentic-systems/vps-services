package system

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// firstText extracts the text of the first content block of a tool result,
// failing the test if the result is nil, has no content, or the first block is
// not a *mcp.TextContent.
//
// Both of our tools return exactly one TextContent block, so this helper keeps
// each assertion focused on behavior rather than on repetitive type-assertion
// boilerplate.
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

// TestHandleHealth verifies that the system_health handler reports a healthy
// status. The handler takes no input and must never error, so we assert a
// successful (non-error) result whose text advertises "status: ok".
func TestHandleHealth(t *testing.T) {
	res, _, err := handleHealth(context.Background(), nil, HealthInput{})
	if err != nil {
		t.Fatalf("handleHealth returned unexpected Go error: %v", err)
	}
	if res.IsError {
		t.Fatalf("handleHealth reported IsError=true, want false; text=%q", firstText(t, res))
	}

	text := firstText(t, res)
	if !strings.Contains(text, "status: ok") {
		t.Errorf("health text = %q, want it to contain %q", text, "status: ok")
	}
}

// TestHandleTime exercises the system_time handler across its three meaningful
// behaviors, using a table so each row documents one contract of the tool.
func TestHandleTime(t *testing.T) {
	tests := []struct {
		name string

		// input is the tool argument under test.
		input TimeInput

		// wantIsError asserts whether the result should be flagged as a
		// tool-level (self-healing) error.
		wantIsError bool

		// wantContains is a substring the result text must contain. For the
		// success cases this is the resolved timezone name; for the error case it
		// is the self-healing guidance prefix.
		wantContains string
	}{
		{
			name:         "omitted timezone defaults to UTC",
			input:        TimeInput{}, // no timezone -> UTC
			wantIsError:  false,
			wantContains: "UTC",
		},
		{
			name:         "valid IANA timezone loads successfully",
			input:        TimeInput{Timezone: "America/New_York"},
			wantIsError:  false,
			wantContains: "America/New_York",
		},
		{
			name:         "invalid timezone returns a self-healing error",
			input:        TimeInput{Timezone: "Not/ARealZone"},
			wantIsError:  true,
			wantContains: "Invalid 'timezone'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, _, err := handleTime(context.Background(), nil, tt.input)

			// The self-healing contract (directive 2.4) is strict: even on bad
			// input the handler must NOT return a Go error, because doing so would
			// surface as a protocol failure instead of readable tool output. The
			// error signal is carried by res.IsError, not by err.
			if err != nil {
				t.Fatalf("handleTime returned a Go error %v, want nil (errors must be conveyed via IsError)", err)
			}

			if res.IsError != tt.wantIsError {
				t.Errorf("IsError = %v, want %v; text=%q", res.IsError, tt.wantIsError, firstText(t, res))
			}

			text := firstText(t, res)
			if !strings.Contains(text, tt.wantContains) {
				t.Errorf("time text = %q, want it to contain %q", text, tt.wantContains)
			}
		})
	}
}
