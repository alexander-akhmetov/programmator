package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessTextOutput(t *testing.T) {
	var collected []string
	opts := InvokeOptions{
		OnOutput: func(text string) {
			collected = append(collected, text)
		},
	}

	input := "line1\nline2\nline3"
	output := processTextOutput(strings.NewReader(input), opts)

	require.Equal(t, "line1\nline2\nline3\n", output)
	require.Len(t, collected, 3)
}

func TestProcessTextOutputNoCallback(t *testing.T) {
	input := "line1\nline2\n"
	output := processTextOutput(strings.NewReader(input), InvokeOptions{})
	require.Equal(t, "line1\nline2\n", output)
}

func TestProcessStreamingOutput(t *testing.T) {
	var collected []string
	opts := InvokeOptions{
		OnOutput: func(text string) {
			collected = append(collected, text)
		},
	}

	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":" World"}]}}
{"type":"result","result":"final"}`

	output := processStreamingOutput(strings.NewReader(input), opts)

	require.Equal(t, "Hello World", output)
	require.Len(t, collected, 2)
}

func TestProcessStreamingOutputEmpty(t *testing.T) {
	input := `{"type":"result","result":"only result"}`
	output := processStreamingOutput(strings.NewReader(input), InvokeOptions{})
	require.Equal(t, "only result", output)
}

func TestProcessStreamingOutputDeduplicatesToolUse(t *testing.T) {
	var toolUses []string
	opts := InvokeOptions{
		OnToolUse: func(name string, _ any) {
			toolUses = append(toolUses, name)
		},
	}

	// Same tool_use ID sent twice (streaming sends cumulative content)
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"t1"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"t1"}]}}
{"type":"result","result":""}`

	processStreamingOutput(strings.NewReader(input), opts)
	require.Len(t, toolUses, 1, "duplicate tool_use blocks should be deduplicated")
}

func TestProcessStreamingOutputSystemInit(t *testing.T) {
	var model string
	opts := InvokeOptions{
		OnSystemInit: func(m string) {
			model = m
		},
	}

	input := `{"type":"system","subtype":"init","model":"claude-3-opus"}
{"type":"result","result":""}`

	processStreamingOutput(strings.NewReader(input), opts)
	require.Equal(t, "claude-3-opus", model)
}

func TestProcessStreamingOutputTokenTracking(t *testing.T) {
	var lastInput, lastOutput int
	var finalModel string
	var finalInput, finalOutput int

	opts := InvokeOptions{
		OnTokens: func(inp, out int) {
			lastInput = inp
			lastOutput = out
		},
		OnFinalTokens: func(m string, inp, out int) {
			finalModel = m
			finalInput = inp
			finalOutput = out
		},
	}

	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hi"}],"usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":5,"cache_read_input_tokens":3}}}
{"type":"result","result":"","modelUsage":{"claude-3":{"inputTokens":200,"outputTokens":50,"cacheCreationInputTokens":10,"cacheReadInputTokens":5}}}`

	processStreamingOutput(strings.NewReader(input), opts)

	require.Equal(t, 108, lastInput) // 100 + 5 + 3
	require.Equal(t, 10, lastOutput)
	require.Equal(t, "claude-3", finalModel)
	require.Equal(t, 215, finalInput) // 200 + 10 + 5
	require.Equal(t, 50, finalOutput)
}

func TestProcessStreamingOutputInvalidJSON(t *testing.T) {
	// Invalid JSON lines should be skipped without error
	input := `not json
{"type":"assistant","message":{"content":[{"type":"text","text":"OK"}]}}
also not json
{"type":"result","result":""}`

	output := processStreamingOutput(strings.NewReader(input), InvokeOptions{})
	require.Equal(t, "OK", output)
}

func TestProcessStreamingOutputToolResult(t *testing.T) {
	var toolName, toolResult string
	opts := InvokeOptions{
		OnToolResult: func(name, result string) {
			toolName = name
			toolResult = result
		},
	}

	input := `{"type":"user","tool_name":"Bash","tool_result":"output here"}
{"type":"result","result":""}`

	processStreamingOutput(strings.NewReader(input), opts)
	require.Equal(t, "Bash", toolName)
	require.Equal(t, "output here", toolResult)
}

func TestProcessStreamingOutputEmptyReader(t *testing.T) {
	output := processStreamingOutput(strings.NewReader(""), InvokeOptions{})
	require.Equal(t, "", output)
}

func TestProcessTextOutputEmptyReader(t *testing.T) {
	output := processTextOutput(strings.NewReader(""), InvokeOptions{})
	require.Equal(t, "", output)
}

func TestProcessStreamingOutputBlankLines(t *testing.T) {
	input := "\n\n  \n{\"type\":\"result\",\"result\":\"ok\"}\n\n"
	output := processStreamingOutput(strings.NewReader(input), InvokeOptions{})
	require.Equal(t, "ok", output)
}

func TestProcessStreamingOutputNilOnToolUse(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"t1","input":{"file_path":"/tmp/test.go"}}]}}
{"type":"result","result":""}`

	output := processStreamingOutput(strings.NewReader(input), InvokeOptions{})
	require.Equal(t, "", output)
}

func TestProcessStreamingOutputToolUseWithoutID(t *testing.T) {
	var toolUses []string
	opts := InvokeOptions{
		OnToolUse: func(name string, _ any) {
			toolUses = append(toolUses, name)
		},
	}

	// tool_use without ID should still be reported (no dedup possible)
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}
{"type":"result","result":""}`

	processStreamingOutput(strings.NewReader(input), opts)
	require.Len(t, toolUses, 2, "tool_use without ID cannot be deduplicated")
}

func TestMessageUsageTotalInputTokens(t *testing.T) {
	u := messageUsage{
		InputTokens:              100,
		CacheCreationInputTokens: 20,
		CacheReadInputTokens:     30,
	}
	require.Equal(t, 150, u.TotalInputTokens())
}

func TestModelUsageStatsTotalInputTokens(t *testing.T) {
	u := modelUsageStats{
		InputTokens:              200,
		CacheCreationInputTokens: 10,
		CacheReadInputTokens:     5,
	}
	require.Equal(t, 215, u.TotalInputTokens())
}

func TestProcessStreamingOutputMultipleModelsInResult(t *testing.T) {
	var finalCalls []string
	opts := InvokeOptions{
		OnFinalTokens: func(m string, _, _ int) {
			finalCalls = append(finalCalls, m)
		},
	}

	input := `{"type":"result","result":"","modelUsage":{"claude-3":{"inputTokens":100,"outputTokens":50},"claude-haiku":{"inputTokens":20,"outputTokens":10}}}`
	processStreamingOutput(strings.NewReader(input), opts)
	require.Len(t, finalCalls, 2)
}

func TestProcessStreamingOutputResultFallback(t *testing.T) {
	// When assistant events produce text, result.result should be ignored
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"from assistant"}]}}
{"type":"result","result":"from result"}`

	output := processStreamingOutput(strings.NewReader(input), InvokeOptions{})
	require.Equal(t, "from assistant", output)
}
