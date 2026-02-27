package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/debug"
)

// piEvent is the JSON structure emitted by `pi --mode json`.
type piEvent struct {
	Type string `json:"type"`

	// message_start / message_update / message_end
	Message *piMessage `json:"message,omitempty"`

	// message_update: streaming delta details
	AssistantMessageEvent *piAssistantMsgEvent `json:"assistantMessageEvent,omitempty"`

	// tool_execution_start / tool_execution_end
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Args       any    `json:"args,omitempty"`
	Result     any    `json:"result,omitempty"`
	IsError    bool   `json:"isError,omitempty"`

	// agent_end: all messages in the session
	Messages []piMessage `json:"messages,omitempty"`
}

// piMessage represents a message in pi's JSON output.
type piMessage struct {
	Role    string      `json:"role"`
	Model   string      `json:"model,omitempty"`
	Content []piContent `json:"content,omitempty"`
	Usage   piUsage     `json:"usage,omitzero"`
}

// piContent represents a content block within a pi message.
type piContent struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// piUsage holds token counts from a pi message.
type piUsage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
}

func (u piUsage) TotalInputTokens() int {
	return u.Input + u.CacheRead + u.CacheWrite
}

// piAssistantMsgEvent holds the streaming delta for a message_update event.
type piAssistantMsgEvent struct {
	Type         string     `json:"type"` // text_delta, toolcall_start, toolcall_end, etc.
	Delta        string     `json:"delta,omitempty"`
	ContentIndex int        `json:"contentIndex,omitempty"`
	ToolCall     *piContent `json:"toolCall,omitempty"`
}

// processPiStreamingOutput reads JSON lines from pi --mode json output,
// dispatches callbacks via opts, and returns the accumulated text output.
func processPiStreamingOutput(r io.Reader, opts InvokeOptions) string {
	var fullOutput strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	processedToolIDs := make(map[string]bool)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event piEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			debug.Logf("pi stream: failed to parse JSON: %v (line: %.100s...)", err, line)
			continue
		}

		debug.Logf("pi stream: event type=%s", event.Type)

		switch event.Type {
		case "session":
			// Session header — no useful callbacks
		case "message_start":
			handlePiMessageStart(&event, opts)
		case "message_update":
			handlePiMessageUpdate(&event, &fullOutput, processedToolIDs, opts)
		case "message_end":
			handlePiMessageEnd(&event, opts)
		case "tool_execution_end":
			handlePiToolExecutionEnd(&event, opts)
		case "agent_end":
			handlePiAgentEnd(&event, opts)
		default:
			debug.Logf("pi stream: unhandled event type=%s", event.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		debug.Logf("pi stream: scanner error: %v", err)
	}

	return fullOutput.String()
}

func handlePiMessageStart(event *piEvent, opts InvokeOptions) {
	if event.Message != nil && event.Message.Role == "assistant" && event.Message.Model != "" {
		if opts.OnSystemInit != nil {
			opts.OnSystemInit(event.Message.Model)
		}
	}
}

func handlePiMessageUpdate(event *piEvent, fullOutput *strings.Builder, processedToolIDs map[string]bool, opts InvokeOptions) {
	if event.AssistantMessageEvent == nil {
		return
	}

	ame := event.AssistantMessageEvent
	switch ame.Type {
	case "text_delta":
		if ame.Delta != "" {
			fullOutput.WriteString(ame.Delta)
			if opts.OnOutput != nil {
				opts.OnOutput(ame.Delta)
			}
		}
	case "toolcall_end":
		tc := ame.ToolCall
		if tc == nil {
			// Fall back to extracting from the message content at contentIndex.
			if event.Message != nil && ame.ContentIndex < len(event.Message.Content) {
				block := &event.Message.Content[ame.ContentIndex]
				tc = block
			}
		}
		if tc != nil && tc.Name != "" {
			if tc.ID != "" && processedToolIDs[tc.ID] {
				return
			}
			if tc.ID != "" {
				processedToolIDs[tc.ID] = true
			}
			if opts.OnToolUse != nil {
				opts.OnToolUse(tc.Name, tc.Arguments)
			}
		}
	}
}

func handlePiMessageEnd(event *piEvent, opts InvokeOptions) {
	if event.Message == nil || opts.OnTokens == nil {
		return
	}
	u := event.Message.Usage
	opts.OnTokens(u.TotalInputTokens(), u.Output)
}

func handlePiToolExecutionEnd(event *piEvent, opts InvokeOptions) {
	if opts.OnToolResult == nil || event.ToolName == "" {
		return
	}
	opts.OnToolResult(event.ToolName, extractToolResultText(event.Result))
}

// extractToolResultText extracts human-readable text from a Pi tool result.
// Pi returns results in two shapes:
//   - {content: [{text: "...", type: "text"}, ...]}           (single object)
//   - [{content: [{text: "...", type: "text"}, ...]}, ...]    (array of objects)
//
// Falls back to JSON marshaling for unknown shapes.
func extractToolResultText(result any) string {
	if result == nil {
		return ""
	}

	// Try plain string first.
	if s, ok := result.(string); ok {
		return s
	}

	// Extract text from content blocks within a single map.
	if texts := extractContentTexts(result); len(texts) > 0 {
		return strings.Join(texts, "\n")
	}

	// Try array of maps.
	if items, ok := result.([]any); ok {
		var texts []string
		for _, item := range items {
			texts = append(texts, extractContentTexts(item)...)
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}

	// Fallback: compact JSON.
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result)
	}
	return string(b)
}

// extractContentTexts extracts text strings from a {content: [{text: "...", type: "text"}]} map.
func extractContentTexts(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	contentList, ok := m["content"].([]any)
	if !ok {
		return nil
	}
	var texts []string
	for _, c := range contentList {
		block, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := block["text"].(string); ok {
			texts = append(texts, text)
		}
	}
	return texts
}

func handlePiAgentEnd(event *piEvent, opts InvokeOptions) {
	if opts.OnFinalTokens == nil || len(event.Messages) == 0 {
		return
	}
	totalInput, totalOutput := 0, 0
	var model string
	for i := range event.Messages {
		msg := &event.Messages[i]
		if msg.Role == "assistant" {
			u := msg.Usage
			totalInput += u.TotalInputTokens()
			totalOutput += u.Output
			if msg.Model != "" {
				model = msg.Model
			}
		}
	}
	if model != "" {
		opts.OnFinalTokens(model, totalInput, totalOutput)
	}
}
