package llm

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/debug"
)

// ocEvent is the top-level nd-JSON structure emitted by `opencode run --format json`.
type ocEvent struct {
	Type string `json:"type"`
	Part ocPart `json:"part"`
}

// ocPart holds the event-specific payload.
type ocPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	Tool     string       `json:"tool,omitempty"`
	State    *ocToolState `json:"state,omitempty"`
	Snapshot string       `json:"snapshot,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	Cost     float64      `json:"cost,omitempty"`
	Tokens   *ocTokens    `json:"tokens,omitempty"`
}

// ocToolState holds tool invocation input and output.
type ocToolState struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

// ocTokens holds token counts from a step_finish event.
type ocTokens struct {
	Total     int     `json:"total"`
	Input     int     `json:"input"`
	Output    int     `json:"output"`
	Reasoning int     `json:"reasoning"`
	Cache     ocCache `json:"cache"`
}

// TotalInputTokens returns input + cache read + cache write.
func (t ocTokens) TotalInputTokens() int {
	return t.Input + t.Cache.Read + t.Cache.Write
}

// ocCache holds cache token counts.
type ocCache struct {
	Read  int `json:"read"`
	Write int `json:"write"`
}

// processOpenCodeStreamingOutput reads nd-JSON lines from opencode --format json output,
// dispatches callbacks via opts, and returns the accumulated text output.
func processOpenCodeStreamingOutput(r io.Reader, model string, opts InvokeOptions) string {
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

		var event ocEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			debug.Logf("opencode stream: failed to parse JSON: %v (line: %.100s...)", err, line)
			continue
		}

		debug.Logf("opencode stream: event type=%s part.type=%s", event.Type, event.Part.Type)

		switch event.Part.Type {
		case "step-start":
			// No-op.
		case "text":
			if event.Part.Text != "" {
				fullOutput.WriteString(event.Part.Text)
				if opts.OnOutput != nil {
					opts.OnOutput(event.Part.Text)
				}
			}
		case "tool":
			if event.Part.Tool != "" && event.Part.State != nil {
				if opts.OnToolUse != nil {
					opts.OnToolUse(event.Part.Tool, event.Part.State.Input)
				}
				if opts.OnToolResult != nil {
					opts.OnToolResult(event.Part.Tool, event.Part.State.Output)
				}
			}
		case "step-finish":
			if event.Part.Tokens != nil {
				tok := event.Part.Tokens
				inp := tok.TotalInputTokens()
				out := tok.Output
				if inp > 0 || out > 0 {
					hasTokens = true
					totalInput += inp
					totalOutput += out
					if opts.OnTokens != nil {
						opts.OnTokens(inp, out)
					}
				}
			}
		default:
			debug.Logf("opencode stream: unhandled part type=%s", event.Part.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		debug.Logf("opencode stream: scanner error: %v", err)
	}

	if hasTokens && opts.OnFinalTokens != nil {
		m := model
		if m == "" {
			m = "opencode"
		}
		opts.OnFinalTokens(m, totalInput, totalOutput)
	}

	return fullOutput.String()
}
