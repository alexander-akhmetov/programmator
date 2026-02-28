package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/alexander-akhmetov/programmator/internal/llm"
)

// streamEvent is the JSON structure emitted by `claude --output-format stream-json`.
type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Model   string `json:"model"`
	Message struct {
		Model   string `json:"model"`
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Name  string `json:"name,omitempty"`
			Input any    `json:"input,omitempty"`
			ID    string `json:"id,omitempty"`
		} `json:"content"`
		Usage messageUsage `json:"usage"`
	} `json:"message"`
	ModelUsage map[string]modelUsageStats `json:"modelUsage"`
	Result     string                     `json:"result"`
	ToolName   string                     `json:"tool_name,omitempty"`
	ToolResult string                     `json:"tool_result,omitempty"`
}

type messageUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u messageUsage) TotalInputTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

type modelUsageStats struct {
	InputTokens              int `json:"inputTokens"`
	OutputTokens             int `json:"outputTokens"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
	CacheReadInputTokens     int `json:"cacheReadInputTokens"`
}

func (u modelUsageStats) TotalInputTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// processStreamingOutput reads stream-json lines from r, dispatches callbacks
// via opts, and returns the accumulated text output.
func processStreamingOutput(r io.Reader, opts llm.InvokeOptions) string {
	var fullOutput strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	processedBlockIDs := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			debug.Logf("stream: failed to parse JSON: %v (line: %.100s...)", err, line)
			continue
		}

		debug.Logf("stream: event type=%s subtype=%s", event.Type, event.Subtype)

		switch event.Type {
		case "system":
			handleSystemEvent(&event, opts)
		case "assistant":
			handleAssistantEvent(&event, &fullOutput, processedBlockIDs, opts)
		case "user":
			handleUserEvent(&event, opts)
		case "result":
			handleResultEvent(&event, &fullOutput, opts)
		default:
			debug.Logf("stream: unhandled event type=%s", event.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		debug.Logf("stream: scanner error: %v", err)
	}

	return fullOutput.String()
}

func handleSystemEvent(event *streamEvent, opts llm.InvokeOptions) {
	if event.Subtype == "init" && event.Model != "" && opts.OnSystemInit != nil {
		opts.OnSystemInit(event.Model)
	}
}

func handleAssistantEvent(event *streamEvent, fullOutput *strings.Builder, processedBlockIDs map[string]bool, opts llm.InvokeOptions) {
	if opts.OnTokens != nil {
		opts.OnTokens(
			event.Message.Usage.TotalInputTokens(),
			event.Message.Usage.OutputTokens,
		)
	}

	for i := range event.Message.Content {
		block := &event.Message.Content[i]
		if block.Type == "text" && block.Text != "" {
			fullOutput.WriteString(block.Text)
			if opts.OnOutput != nil {
				opts.OnOutput(block.Text)
			}
		} else if block.Type == "tool_use" && block.Name != "" {
			if block.ID != "" && processedBlockIDs[block.ID] {
				continue
			}
			if block.ID != "" {
				processedBlockIDs[block.ID] = true
			}
			if opts.OnToolUse != nil {
				opts.OnToolUse(block.Name, block.Input)
			}
		}
	}
}

func handleUserEvent(event *streamEvent, opts llm.InvokeOptions) {
	if opts.OnToolResult != nil && event.ToolName != "" {
		opts.OnToolResult(event.ToolName, event.ToolResult)
	}
}

func handleResultEvent(event *streamEvent, fullOutput *strings.Builder, opts llm.InvokeOptions) {
	if opts.OnFinalTokens != nil && len(event.ModelUsage) > 0 {
		for model, usage := range event.ModelUsage {
			opts.OnFinalTokens(model, usage.TotalInputTokens(), usage.OutputTokens)
		}
	}
	if event.Result != "" && fullOutput.Len() == 0 {
		fullOutput.WriteString(event.Result)
	}
}
