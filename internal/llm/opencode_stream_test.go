package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessOpenCodeStreamingOutput(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		model           string
		wantOutput      string
		wantOnOutput    []string
		wantOnTokens    [][2]int // {input, output} per call
		wantFinalModel  string
		wantFinalInput  int
		wantFinalOutput int
		wantFinalCalled bool
		wantToolUses    []string
		wantToolInputs  []string
		wantToolResults []string
	}{
		{
			name: "text accumulation",
			input: `{"type":"step_start","part":{"type":"step-start","snapshot":"abc"}}
{"type":"text","part":{"type":"text","text":"Hello World"}}
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":33,"input":20,"output":13,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			model:           "anthropic/claude-sonnet-4-5",
			wantOutput:      "Hello World",
			wantOnOutput:    []string{"Hello World"},
			wantOnTokens:    [][2]int{{20, 13}},
			wantFinalModel:  "anthropic/claude-sonnet-4-5",
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
			name:            "per-step token tracking with cache",
			input:           `{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":100,"input":50,"output":30,"reasoning":0,"cache":{"read":10,"write":5}}}}`,
			model:           "test-model",
			wantOutput:      "",
			wantOnTokens:    [][2]int{{65, 30}}, // 50 + 10 + 5 = 65
			wantFinalModel:  "test-model",
			wantFinalInput:  65,
			wantFinalOutput: 30,
			wantFinalCalled: true,
		},
		{
			name: "final token aggregation across steps",
			input: `{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":50,"input":20,"output":10,"reasoning":0,"cache":{"read":5,"write":3}}}}
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":80,"input":40,"output":20,"reasoning":0,"cache":{"read":8,"write":2}}}}`,
			model:           "test-model",
			wantOutput:      "",
			wantOnTokens:    [][2]int{{28, 10}, {50, 20}}, // first: 20+5+3=28, second: 40+8+2=50
			wantFinalModel:  "test-model",
			wantFinalInput:  78, // 28 + 50
			wantFinalOutput: 30, // 10 + 20
			wantFinalCalled: true,
		},
		{
			name:            "fallback model name when empty",
			input:           `{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":10,"input":5,"output":5,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			model:           "",
			wantOutput:      "",
			wantOnTokens:    [][2]int{{5, 5}},
			wantFinalModel:  "opencode",
			wantFinalInput:  5,
			wantFinalOutput: 5,
			wantFinalCalled: true,
		},
		{
			name: "tool use event",
			input: `{"type":"tool_use","part":{"type":"tool","tool":"Bash","state":{"input":"{\"command\":\"ls\"}","output":"file1.go\nfile2.go"}}}
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":10,"input":5,"output":5,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			model:           "test-model",
			wantOutput:      "",
			wantToolUses:    []string{"Bash"},
			wantToolInputs:  []string{"{\"command\":\"ls\"}"},
			wantToolResults: []string{"file1.go\nfile2.go"},
			wantFinalCalled: true,
			wantFinalModel:  "test-model",
			wantFinalInput:  5,
			wantFinalOutput: 5,
			wantOnTokens:    [][2]int{{5, 5}},
		},
		{
			name: "multi-step sequence: tool then text",
			input: `{"type":"step_start","part":{"type":"step-start","snapshot":"s1"}}
{"type":"tool_use","part":{"type":"tool","tool":"Read","state":{"input":"test.go","output":"contents"}}}
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":20,"input":10,"output":10,"reasoning":0,"cache":{"read":0,"write":0}}}}
{"type":"step_start","part":{"type":"step-start","snapshot":"s2"}}
{"type":"text","part":{"type":"text","text":"Analysis complete"}}
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":30,"input":15,"output":15,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			model:           "test-model",
			wantOutput:      "Analysis complete",
			wantOnOutput:    []string{"Analysis complete"},
			wantToolUses:    []string{"Read"},
			wantToolInputs:  []string{"test.go"},
			wantToolResults: []string{"contents"},
			wantOnTokens:    [][2]int{{10, 10}, {15, 15}},
			wantFinalModel:  "test-model",
			wantFinalInput:  25,
			wantFinalOutput: 25,
			wantFinalCalled: true,
		},
		{
			name: "invalid JSON lines gracefully skipped",
			input: `not json at all
{"type":"text","part":{"type":"text","text":"valid"}}
{broken json
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":10,"input":5,"output":5,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			model:           "test-model",
			wantOutput:      "valid",
			wantOnOutput:    []string{"valid"},
			wantFinalCalled: true,
			wantFinalModel:  "test-model",
			wantFinalInput:  5,
			wantFinalOutput: 5,
			wantOnTokens:    [][2]int{{5, 5}},
		},
		{
			name:            "blank lines ignored",
			input:           "\n\n  \n{\"type\":\"text\",\"part\":{\"type\":\"text\",\"text\":\"ok\"}}\n\n",
			model:           "test-model",
			wantOutput:      "ok",
			wantOnOutput:    []string{"ok"},
			wantFinalCalled: false,
		},
		{
			name:            "no tokens in step_finish means OnFinalTokens not called",
			input:           `{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":0,"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}}}}`,
			model:           "test-model",
			wantOutput:      "",
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
			var toolInputs []string
			var toolResults []string

			opts := InvokeOptions{
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
				OnToolUse: func(name string, input any) {
					toolUses = append(toolUses, name)
					if s, ok := input.(string); ok {
						toolInputs = append(toolInputs, s)
					}
				},
				OnToolResult: func(_, result string) {
					toolResults = append(toolResults, result)
				},
			}

			output := processOpenCodeStreamingOutput(strings.NewReader(tc.input), tc.model, opts)
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
			if tc.wantToolInputs != nil {
				require.Equal(t, tc.wantToolInputs, toolInputs)
			}
			if tc.wantToolResults != nil {
				require.Equal(t, tc.wantToolResults, toolResults)
			}
		})
	}
}

func TestProcessOpenCodeStreamingOutputNilCallbacks(t *testing.T) {
	input := `{"type":"step_start","part":{"type":"step-start","snapshot":"abc"}}
{"type":"text","part":{"type":"text","text":"hello"}}
{"type":"tool_use","part":{"type":"tool","tool":"Bash","state":{"input":"ls","output":"ok"}}}
{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","tokens":{"total":30,"input":20,"output":10,"reasoning":0,"cache":{"read":0,"write":0}}}}`

	// All callbacks nil — should not panic
	output := processOpenCodeStreamingOutput(strings.NewReader(input), "test", InvokeOptions{})
	require.Equal(t, "hello", output)
}

func TestOCTokensTotalInputTokens(t *testing.T) {
	tok := ocTokens{
		Total:     200,
		Input:     100,
		Output:    50,
		Reasoning: 10,
		Cache: ocCache{
			Read:  20,
			Write: 30,
		},
	}
	require.Equal(t, 150, tok.TotalInputTokens()) // 100 + 20 + 30
}
