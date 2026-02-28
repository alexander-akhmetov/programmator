package pi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/llm"
)

func TestProcessPiStreamingOutput(t *testing.T) {
	var collected []string
	opts := llm.InvokeOptions{
		OnOutput: func(text string) {
			collected = append(collected, text)
		},
	}

	input := `{"type":"session","version":3,"id":"abc","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}
{"type":"message_start","message":{"role":"assistant","model":"claude-sonnet","content":[]}}
{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"text_delta","delta":"Hello "}}
{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"text_delta","delta":"World"}}
{"type":"message_end","message":{"role":"assistant","usage":{"input":100,"output":10,"cacheRead":5,"cacheWrite":3}}}
{"type":"agent_end","messages":[]}`

	output := processPiStreamingOutput(strings.NewReader(input), opts)

	require.Equal(t, "Hello World", output)
	require.Len(t, collected, 2)
	require.Equal(t, "Hello ", collected[0])
	require.Equal(t, "World", collected[1])
}

func TestProcessPiStreamingOutputEmpty(t *testing.T) {
	output := processPiStreamingOutput(strings.NewReader(""), llm.InvokeOptions{})
	require.Equal(t, "", output)
}

func TestProcessPiStreamingOutputSystemInit(t *testing.T) {
	var model string
	opts := llm.InvokeOptions{
		OnSystemInit: func(m string) {
			model = m
		},
	}

	input := `{"type":"message_start","message":{"role":"assistant","model":"claude-sonnet-4","content":[]}}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)
	require.Equal(t, "claude-sonnet-4", model)
}

func TestProcessPiStreamingOutputSystemInitSkipsNonAssistant(t *testing.T) {
	var model string
	opts := llm.InvokeOptions{
		OnSystemInit: func(m string) {
			model = m
		},
	}

	input := `{"type":"message_start","message":{"role":"user","model":"should-not-appear","content":[]}}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)
	require.Equal(t, "", model)
}

func TestProcessPiStreamingOutputTokenTracking(t *testing.T) {
	var lastInput, lastOutput int
	opts := llm.InvokeOptions{
		OnTokens: func(inp, out int) {
			lastInput = inp
			lastOutput = out
		},
	}

	input := `{"type":"message_end","message":{"role":"assistant","usage":{"input":100,"output":50,"cacheRead":10,"cacheWrite":5}}}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)

	require.Equal(t, 115, lastInput) // 100 + 10 + 5
	require.Equal(t, 50, lastOutput)
}

func TestProcessPiStreamingOutputFinalTokens(t *testing.T) {
	var finalModel string
	var finalInput, finalOutput int
	opts := llm.InvokeOptions{
		OnFinalTokens: func(m string, inp, out int) {
			finalModel = m
			finalInput = inp
			finalOutput = out
		},
	}

	input := `{"type":"agent_end","messages":[{"role":"user","content":[]},{"role":"assistant","model":"claude-sonnet","usage":{"input":100,"output":30,"cacheRead":5,"cacheWrite":3}},{"role":"assistant","model":"claude-sonnet","usage":{"input":200,"output":40,"cacheRead":10,"cacheWrite":2}}]}`

	processPiStreamingOutput(strings.NewReader(input), opts)

	require.Equal(t, "claude-sonnet", finalModel)
	require.Equal(t, 320, finalInput) // (100+5+3) + (200+10+2)
	require.Equal(t, 70, finalOutput) // 30 + 40
}

func TestProcessPiStreamingOutputToolUseViaToolCall(t *testing.T) {
	var toolUses []string
	var toolArgs []any
	opts := llm.InvokeOptions{
		OnToolUse: func(name string, input any) {
			toolUses = append(toolUses, name)
			toolArgs = append(toolArgs, input)
		},
	}

	input := `{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"toolcall_end","toolCall":{"type":"toolCall","id":"t1","name":"Bash","arguments":{"command":"ls"}}}}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)
	require.Len(t, toolUses, 1)
	require.Equal(t, "Bash", toolUses[0])
	args, ok := toolArgs[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "ls", args["command"])
}

func TestProcessPiStreamingOutputToolUseViaContentIndex(t *testing.T) {
	var toolUses []string
	opts := llm.InvokeOptions{
		OnToolUse: func(name string, _ any) {
			toolUses = append(toolUses, name)
		},
	}

	input := `{"type":"message_update","message":{"role":"assistant","content":[{"type":"toolCall","id":"t1","name":"Read","arguments":{"file_path":"/tmp/test.go"}}]},"assistantMessageEvent":{"type":"toolcall_end","contentIndex":0}}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)
	require.Len(t, toolUses, 1)
	require.Equal(t, "Read", toolUses[0])
}

func TestProcessPiStreamingOutputDeduplicatesToolUse(t *testing.T) {
	var toolUses []string
	opts := llm.InvokeOptions{
		OnToolUse: func(name string, _ any) {
			toolUses = append(toolUses, name)
		},
	}

	// Same tool ID sent twice
	input := `{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"toolcall_end","toolCall":{"type":"toolCall","id":"t1","name":"Bash","arguments":{}}}}
{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"toolcall_end","toolCall":{"type":"toolCall","id":"t1","name":"Bash","arguments":{}}}}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)
	require.Len(t, toolUses, 1, "duplicate tool calls should be deduplicated")
}

func TestProcessPiStreamingOutputToolResult(t *testing.T) {
	var toolName, toolResult string
	opts := llm.InvokeOptions{
		OnToolResult: func(name, result string) {
			toolName = name
			toolResult = result
		},
	}

	input := `{"type":"tool_execution_end","toolCallId":"call_123","toolName":"Bash","result":"output here","isError":false}
{"type":"agent_end","messages":[]}`

	processPiStreamingOutput(strings.NewReader(input), opts)
	require.Equal(t, "Bash", toolName)
	require.Equal(t, "output here", toolResult)
}

func TestProcessPiStreamingOutputInvalidJSON(t *testing.T) {
	input := `not json
{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"text_delta","delta":"OK"}}
also not json
{"type":"agent_end","messages":[]}`

	output := processPiStreamingOutput(strings.NewReader(input), llm.InvokeOptions{})
	require.Equal(t, "OK", output)
}

func TestProcessPiStreamingOutputBlankLines(t *testing.T) {
	input := "\n\n  \n{\"type\":\"message_update\",\"message\":{\"role\":\"assistant\"},\"assistantMessageEvent\":{\"type\":\"text_delta\",\"delta\":\"ok\"}}\n\n"
	output := processPiStreamingOutput(strings.NewReader(input), llm.InvokeOptions{})
	require.Equal(t, "ok", output)
}

func TestProcessPiStreamingOutputNoCallbacks(t *testing.T) {
	input := `{"type":"session","version":3,"id":"abc"}
{"type":"message_start","message":{"role":"assistant","model":"test"}}
{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"text_delta","delta":"hello"}}
{"type":"message_end","message":{"role":"assistant","usage":{"input":10,"output":5}}}
{"type":"tool_execution_end","toolCallId":"c1","toolName":"Bash","result":"ok"}
{"type":"agent_end","messages":[{"role":"assistant","model":"test","usage":{"input":10,"output":5}}]}`

	output := processPiStreamingOutput(strings.NewReader(input), llm.InvokeOptions{})
	require.Equal(t, "hello", output)
}

func TestPiUsageTotalInputTokens(t *testing.T) {
	u := piUsage{
		Input:      100,
		CacheRead:  20,
		CacheWrite: 30,
	}
	require.Equal(t, 150, u.TotalInputTokens())
}

func TestProcessPiStreamingOutputAgentEndNoAssistant(t *testing.T) {
	var called bool
	opts := llm.InvokeOptions{
		OnFinalTokens: func(_ string, _, _ int) {
			called = true
		},
	}

	// agent_end with only user messages -- no model to report
	input := `{"type":"agent_end","messages":[{"role":"user","content":[]}]}`
	processPiStreamingOutput(strings.NewReader(input), opts)
	require.False(t, called, "OnFinalTokens should not be called when no assistant messages have a model")
}

func TestExtractToolResultText(t *testing.T) {
	tests := []struct {
		name   string
		result any
		want   string
	}{
		{name: "nil", result: nil, want: ""},
		{name: "string", result: "hello", want: "hello"},
		{
			name: "single object with content",
			result: map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "line1"},
					map[string]any{"type": "text", "text": "line2"},
				},
			},
			want: "line1\nline2",
		},
		{
			name: "array of objects",
			result: []any{
				map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "from first"},
					},
				},
				map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "from second"},
					},
				},
			},
			want: "from first\nfrom second",
		},
		{
			name:   "unknown shape falls back to JSON",
			result: map[string]any{"key": "value"},
			want:   `{"key":"value"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractToolResultText(tc.result)
			require.Equal(t, tc.want, got)
		})
	}
}
