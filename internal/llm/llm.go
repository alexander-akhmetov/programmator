// Package llm centralises all Claude CLI invocation, streaming JSON parsing,
// usage accounting, and hook-settings building.
package llm

import (
	"context"
)

// Invoker runs a Claude CLI invocation and returns the text output plus
// aggregated token usage.
type Invoker interface {
	// Invoke sends the prompt to Claude and returns the text output.
	// The callbacks on opts are called during streaming.
	Invoke(ctx context.Context, prompt string, opts InvokeOptions) (*InvokeResult, error)
}

// InvokeOptions configures a single Claude invocation.
type InvokeOptions struct {
	// WorkingDir for the Claude subprocess.
	WorkingDir string

	// Streaming enables stream-json output mode.
	Streaming bool

	// ExtraFlags are additional CLI flags appended to the command.
	ExtraFlags string

	// SettingsJSON is the --settings payload (hook configuration).
	SettingsJSON string

	// Timeout overrides the default invocation timeout (seconds).
	// Zero means no explicit timeout (caller's context is respected).
	Timeout int

	// OnOutput is called with text fragments as they arrive.
	OnOutput func(text string)

	// OnToolUse is called when a tool_use block is observed (streaming).
	OnToolUse func(name string, input any)

	// OnToolResult is called when a tool result is observed (streaming).
	OnToolResult func(toolName, result string)

	// OnSystemInit is called when a system init event provides the model name.
	OnSystemInit func(model string)

	// OnTokens is called with live token counts during streaming.
	OnTokens func(inputTokens, outputTokens int)

	// OnFinalTokens is called with per-model final token counts.
	OnFinalTokens func(model string, inputTokens, outputTokens int)

	// OnProcessStart is called with the PID when the Claude process starts.
	// OnProcessEnd is called when the process exits.
	OnProcessStart func(pid int)
	OnProcessEnd   func()
}

// InvokeResult holds the output of a completed invocation.
type InvokeResult struct {
	// Text is the full text output from Claude.
	Text string
}
