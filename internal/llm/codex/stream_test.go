package codex

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/llm"
)

func TestProcessCodexStreamingOutput(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		model           string
		wantOutput      string
		wantOnOutput    []string
		wantOnTokens    [][2]int
		wantFinalModel  string
		wantFinalInput  int
		wantFinalOutput int
		wantFinalCalled bool
		wantToolUses    []string
		wantToolResults []string
	}{
		{
			name: "agent message text accumulation",
			input: `{"type":"thread.started","thread_id":"t1"}
{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"Hello World"}}
{"type":"turn.completed","usage":{"input_tokens":20,"cached_input_tokens":0,"output_tokens":13}}`,
			model:           "o3",
			wantOutput:      "Hello World",
			wantOnOutput:    []string{"Hello World"},
			wantOnTokens:    [][2]int{{20, 13}},
			wantFinalModel:  "o3",
			wantFinalInput:  20,
			wantFinalOutput: 13,
			wantFinalCalled: true,
		},
		{
			name:            "empty input",
			input:           "",
			model:           "test-model",
			wantOutput:      "",
			wantFinalCalled: false,
		},
		{
			name:            "token tracking with cached tokens",
			input:           `{"type":"turn.completed","usage":{"input_tokens":50,"cached_input_tokens":15,"output_tokens":30}}`,
			model:           "o3",
			wantOutput:      "",
			wantOnTokens:    [][2]int{{65, 30}},
			wantFinalModel:  "o3",
			wantFinalInput:  65,
			wantFinalOutput: 30,
			wantFinalCalled: true,
		},
		{
			name: "multi-turn token aggregation",
			input: `{"type":"turn.completed","usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":10}}
{"type":"turn.completed","usage":{"input_tokens":40,"cached_input_tokens":8,"output_tokens":20}}`,
			model:           "o3",
			wantOutput:      "",
			wantOnTokens:    [][2]int{{25, 10}, {48, 20}},
			wantFinalModel:  "o3",
			wantFinalInput:  73,
			wantFinalOutput: 30,
			wantFinalCalled: true,
		},
		{
			name:            "fallback model name when empty",
			input:           `{"type":"turn.completed","usage":{"input_tokens":5,"cached_input_tokens":0,"output_tokens":5}}`,
			model:           "",
			wantOutput:      "",
			wantOnTokens:    [][2]int{{5, 5}},
			wantFinalModel:  "codex",
			wantFinalInput:  5,
			wantFinalOutput: 5,
			wantFinalCalled: true,
		},
		{
			name: "command execution tool use and result",
			input: `{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"ls -la","aggregated_output":"file1.go\nfile2.go","exit_code":0}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":5}}`,
			model:           "o3",
			wantOutput:      "",
			wantToolUses:    []string{"ls -la"},
			wantToolResults: []string{"file1.go\nfile2.go"},
			wantFinalCalled: true,
			wantFinalModel:  "o3",
			wantFinalInput:  10,
			wantFinalOutput: 5,
			wantOnTokens:    [][2]int{{10, 5}},
		},
		{
			name:            "mcp tool call with server",
			input:           `{"type":"item.completed","item":{"id":"i1","type":"mcp_tool_call","server":"myserver","tool":"search"}}`,
			model:           "o3",
			wantOutput:      "",
			wantToolUses:    []string{"myserver/search"},
			wantFinalCalled: false,
		},
		{
			name:            "mcp tool call without server",
			input:           `{"type":"item.completed","item":{"id":"i1","type":"mcp_tool_call","tool":"search"}}`,
			model:           "o3",
			wantOutput:      "",
			wantToolUses:    []string{"search"},
			wantFinalCalled: false,
		},
		{
			name:            "file change event",
			input:           `{"type":"item.completed","item":{"id":"i1","type":"file_change"}}`,
			model:           "o3",
			wantOutput:      "",
			wantToolUses:    []string{"file_change"},
			wantFinalCalled: false,
		},
		{
			name: "multi-step: tool then text",
			input: `{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"cat main.go","aggregated_output":"package main"}}
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":0,"output_tokens":10}}
{"type":"item.completed","item":{"id":"i2","type":"agent_message","text":"Analysis complete"}}
{"type":"turn.completed","usage":{"input_tokens":15,"cached_input_tokens":0,"output_tokens":15}}`,
			model:           "o3",
			wantOutput:      "Analysis complete",
			wantOnOutput:    []string{"Analysis complete"},
			wantToolUses:    []string{"cat main.go"},
			wantToolResults: []string{"package main"},
			wantOnTokens:    [][2]int{{10, 10}, {15, 15}},
			wantFinalModel:  "o3",
			wantFinalInput:  25,
			wantFinalOutput: 25,
			wantFinalCalled: true,
		},
		{
			name: "invalid JSON lines gracefully skipped",
			input: `not json at all
{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"valid"}}
{broken json
{"type":"turn.completed","usage":{"input_tokens":5,"cached_input_tokens":0,"output_tokens":5}}`,
			model:           "o3",
			wantOutput:      "valid",
			wantOnOutput:    []string{"valid"},
			wantFinalCalled: true,
			wantFinalModel:  "o3",
			wantFinalInput:  5,
			wantFinalOutput: 5,
			wantOnTokens:    [][2]int{{5, 5}},
		},
		{
			name:            "blank lines ignored",
			input:           "\n\n  \n{\"type\":\"item.completed\",\"item\":{\"id\":\"i1\",\"type\":\"agent_message\",\"text\":\"ok\"}}\n\n",
			model:           "o3",
			wantOutput:      "ok",
			wantOnOutput:    []string{"ok"},
			wantFinalCalled: false,
		},
		{
			name:            "zero tokens means OnFinalTokens not called",
			input:           `{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0}}`,
			model:           "o3",
			wantOutput:      "",
			wantFinalCalled: false,
		},
		{
			name: "item.started and item.updated are ignored",
			input: `{"type":"item.started","item":{"id":"i1","type":"agent_message","text":"partial"}}
{"type":"item.updated","item":{"id":"i1","type":"agent_message","text":"more partial"}}
{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"final"}}`,
			model:           "o3",
			wantOutput:      "final",
			wantOnOutput:    []string{"final"},
			wantFinalCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var outputCollected []string
			var tokenCalls [][2]int
			var finalModel string
			var finalInput, finalOutput int
			var finalCalled bool
			var toolUses []string
			var toolResults []string

			opts := llm.InvokeOptions{
				OnOutput: func(text string) {
					outputCollected = append(outputCollected, text)
				},
				OnTokens: func(inp, out int) {
					tokenCalls = append(tokenCalls, [2]int{inp, out})
				},
				OnFinalTokens: func(m string, inp, out int) {
					finalCalled = true
					finalModel = m
					finalInput = inp
					finalOutput = out
				},
				OnToolUse: func(name string, _ any) {
					toolUses = append(toolUses, name)
				},
				OnToolResult: func(_, result string) {
					toolResults = append(toolResults, result)
				},
			}

			output := processCodexStreamingOutput(strings.NewReader(tc.input), tc.model, opts)
			require.Equal(t, tc.wantOutput, output)

			if tc.wantOnOutput != nil {
				require.Equal(t, tc.wantOnOutput, outputCollected)
			}

			if tc.wantOnTokens != nil {
				require.Equal(t, tc.wantOnTokens, tokenCalls)
			}

			require.Equal(t, tc.wantFinalCalled, finalCalled, "OnFinalTokens called mismatch")
			if tc.wantFinalCalled {
				require.Equal(t, tc.wantFinalModel, finalModel)
				require.Equal(t, tc.wantFinalInput, finalInput)
				require.Equal(t, tc.wantFinalOutput, finalOutput)
			}

			if tc.wantToolUses != nil {
				require.Equal(t, tc.wantToolUses, toolUses)
			}
			if tc.wantToolResults != nil {
				require.Equal(t, tc.wantToolResults, toolResults)
			}
		})
	}
}

func TestProcessCodexStreamingOutputNilCallbacks(t *testing.T) {
	input := `{"type":"thread.started","thread_id":"t1"}
{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"hello"}}
{"type":"item.completed","item":{"id":"i2","type":"command_execution","command":"ls","aggregated_output":"ok"}}
{"type":"item.completed","item":{"id":"i3","type":"mcp_tool_call","server":"s","tool":"t"}}
{"type":"item.completed","item":{"id":"i4","type":"file_change"}}
{"type":"turn.completed","usage":{"input_tokens":20,"cached_input_tokens":0,"output_tokens":10}}`

	output := processCodexStreamingOutput(strings.NewReader(input), "test", llm.InvokeOptions{})
	require.Equal(t, "hello", output)
}
