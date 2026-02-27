package llm

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestClaudeInvokerTextMode(t *testing.T) {
	// Create a fake claude binary that echoes stdin
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	inv := NewClaudeInvoker(EnvConfig{})
	var collected []string
	opts := InvokeOptions{
		OnOutput: func(text string) {
			collected = append(collected, text)
		},
	}

	res, err := inv.Invoke(context.Background(), "hello world", opts)
	require.NoError(t, err)
	require.Equal(t, "hello world\n", res.Text)
	require.Len(t, collected, 1)
}

func TestClaudeInvokerWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Script that prints the current working directory
	script := "#!/bin/sh\ncat >/dev/null\npwd\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	workDir := t.TempDir()
	inv := NewClaudeInvoker(EnvConfig{})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{WorkingDir: workDir})
	require.NoError(t, err)
	require.Contains(t, res.Text, workDir)
}

func TestClaudeInvokerStreamingMode(t *testing.T) {
	tmpDir := t.TempDir()
	// Script outputs stream-json events
	script := `#!/bin/sh
cat >/dev/null
echo '{"type":"system","subtype":"init","model":"test-model"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}],"usage":{"input_tokens":10,"output_tokens":5}}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":" World"}],"usage":{"input_tokens":10,"output_tokens":8}}}'
echo '{"type":"result","result":"final","modelUsage":{"test-model":{"inputTokens":20,"outputTokens":13}}}'
`
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	inv := NewClaudeInvoker(EnvConfig{})

	var textCollected []string
	var model string
	var finalModel string
	var finalInput, finalOutput int
	opts := InvokeOptions{
		Streaming: true,
		OnOutput: func(text string) {
			textCollected = append(textCollected, text)
		},
		OnSystemInit: func(m string) {
			model = m
		},
		OnFinalTokens: func(m string, inp, out int) {
			finalModel = m
			finalInput = inp
			finalOutput = out
		},
	}

	res, err := inv.Invoke(context.Background(), "test prompt", opts)
	require.NoError(t, err)
	require.Equal(t, "Hello World", res.Text)
	require.Equal(t, "test-model", model)
	require.Equal(t, "test-model", finalModel)
	require.Equal(t, 20, finalInput)
	require.Equal(t, 13, finalOutput)
	require.Len(t, textCollected, 2) // "Hello" and " World"
}

func TestClaudeInvokerErrorCapturesStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho 'some error' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	inv := NewClaudeInvoker(EnvConfig{})
	_, err = inv.Invoke(context.Background(), "test", InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "claude exited")
	require.Contains(t, err.Error(), "some error")
}

func TestClaudeInvokerErrorWithoutStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\nexit 1\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	inv := NewClaudeInvoker(EnvConfig{})
	_, err = inv.Invoke(context.Background(), "test", InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "claude exited")
	require.NotContains(t, err.Error(), "stderr")
}

func TestClaudeInvokerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\nsleep 30\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	inv := NewClaudeInvoker(EnvConfig{})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{Timeout: 1})
	require.NoError(t, err) // timeout returns a blocked status, not an error
	require.Contains(t, res.Text, protocol.StatusBlockKey)
	require.Contains(t, res.Text, string(protocol.StatusBlocked))
}

func TestClaudeInvokerToolUseCallback(t *testing.T) {
	tmpDir := t.TempDir()
	script := `#!/bin/sh
cat >/dev/null
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"t1","input":{"file_path":"/tmp/test.go"}}],"usage":{}}}'
echo '{"type":"user","tool_name":"Read","tool_result":"file contents here"}'
echo '{"type":"result","result":""}'
`
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+":"+origPath)

	inv := NewClaudeInvoker(EnvConfig{})

	var toolName string
	var toolInput any
	var toolResultName, toolResultContent string
	opts := InvokeOptions{
		Streaming: true,
		OnToolUse: func(name string, input any) {
			toolName = name
			toolInput = input
		},
		OnToolResult: func(name, result string) {
			toolResultName = name
			toolResultContent = result
		},
	}

	_, err = inv.Invoke(context.Background(), "test", opts)
	require.NoError(t, err)
	require.Equal(t, "Read", toolName)
	require.Equal(t, "Read", toolResultName)
	require.Equal(t, "file contents here", toolResultContent)
	inputMap, ok := toolInput.(map[string]any)
	require.True(t, ok, "tool input should be a map")
	require.Equal(t, "/tmp/test.go", inputMap["file_path"])
}
