package codex

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/alexander-akhmetov/programmator/internal/llm"
)

// codexEvent is the top-level JSONL structure emitted by `codex exec --json`.
type codexEvent struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id,omitempty"`
	Usage    *codexUsage     `json:"usage,omitempty"`
	Error    *codexError     `json:"error,omitempty"`
	Item     json.RawMessage `json:"item,omitempty"`
	Message  string          `json:"message,omitempty"`
}

// codexUsage holds token counts from a turn.completed event.
type codexUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

// codexError holds an error message from a turn.failed event.
type codexError struct {
	Message string `json:"message"`
}

// codexItem holds the payload for item.completed events.
type codexItem struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	Command          string `json:"command,omitempty"`
	AggregatedOutput string `json:"aggregated_output,omitempty"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Server           string `json:"server,omitempty"`
	Tool             string `json:"tool,omitempty"`
}

// processCodexStreamingOutput reads JSONL lines from codex exec --json output,
// dispatches callbacks via opts, and returns the accumulated text output.
func processCodexStreamingOutput(r io.Reader, model string, opts llm.InvokeOptions) string {
	var fullOutput strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	totalInput, totalOutput := 0, 0
	hasTokens := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event codexEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			debug.Logf("codex stream: failed to parse JSON: %v (line: %.100s...)", err, line)
			continue
		}

		debug.Logf("codex stream: event type=%s", event.Type)

		switch event.Type {
		case "item.completed":
			processItemCompleted(event.Item, &fullOutput, opts)

		case "turn.completed":
			if event.Usage != nil {
				inp := event.Usage.InputTokens + event.Usage.CachedInputTokens
				out := event.Usage.OutputTokens
				if inp > 0 || out > 0 {
					hasTokens = true
					totalInput += inp
					totalOutput += out
					if opts.OnTokens != nil {
						opts.OnTokens(inp, out)
					}
				}
			}

		case "thread.started", "turn.started", "item.started", "item.updated":
			// No-op for these event types.

		case "turn.failed":
			if event.Error != nil {
				debug.Logf("codex stream: turn failed: %s", event.Error.Message)
			}

		case "error":
			debug.Logf("codex stream: error event: %s", event.Message)

		default:
			debug.Logf("codex stream: unhandled event type=%s", event.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		debug.Logf("codex stream: scanner error: %v", err)
	}

	if hasTokens && opts.OnFinalTokens != nil {
		m := model
		if m == "" {
			m = "codex"
		}
		opts.OnFinalTokens(m, totalInput, totalOutput)
	}

	return fullOutput.String()
}

// processItemCompleted handles item.completed events by dispatching to the
// appropriate callback based on the item type.
func processItemCompleted(raw json.RawMessage, fullOutput *strings.Builder, opts llm.InvokeOptions) {
	if len(raw) == 0 {
		return
	}

	var item codexItem
	if err := json.Unmarshal(raw, &item); err != nil {
		debug.Logf("codex stream: failed to parse item: %v", err)
		return
	}

	switch item.Type {
	case "agent_message":
		if item.Text != "" {
			fullOutput.WriteString(item.Text)
			if opts.OnOutput != nil {
				opts.OnOutput(item.Text)
			}
		}

	case "command_execution":
		if opts.OnToolUse != nil {
			opts.OnToolUse(item.Command, item.Command)
		}
		if opts.OnToolResult != nil {
			opts.OnToolResult(item.Command, item.AggregatedOutput)
		}

	case "mcp_tool_call":
		if opts.OnToolUse != nil {
			name := item.Tool
			if item.Server != "" {
				name = item.Server + "/" + item.Tool
			}
			opts.OnToolUse(name, name)
		}

	case "file_change":
		if opts.OnToolUse != nil {
			opts.OnToolUse("file_change", "file_change")
		}

	default:
		debug.Logf("codex stream: unhandled item type=%s", item.Type)
	}
}
