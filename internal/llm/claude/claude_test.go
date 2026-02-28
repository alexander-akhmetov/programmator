package claude

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestInvokerTextMode(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	var collected []string
	opts := llm.InvokeOptions{
		OnOutput: func(text string) {
			collected = append(collected, text)
		},
	}

	res, err := inv.Invoke(context.Background(), "hello world", opts)
	require.NoError(t, err)
	require.Equal(t, "hello world\n", res.Text)
	require.Len(t, collected, 1)
}

func TestInvokerWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\npwd\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	workDir := t.TempDir()
	inv := New(Config{})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{WorkingDir: workDir})
	require.NoError(t, err)
	require.Contains(t, res.Text, workDir)
}

func TestInvokerStreamingMode(t *testing.T) {
	tmpDir := t.TempDir()
	script := `#!/bin/sh
cat >/dev/null
echo '{"type":"system","subtype":"init","model":"test-model"}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}],"usage":{"input_tokens":10,"output_tokens":5}}}'
echo '{"type":"assistant","message":{"content":[{"type":"text","text":" World"}],"usage":{"input_tokens":10,"output_tokens":8}}}'
echo '{"type":"result","result":"final","modelUsage":{"test-model":{"inputTokens":20,"outputTokens":13}}}'
`
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})

	var textCollected []string
	var model string
	var finalModel string
	var finalInput, finalOutput int
	opts := llm.InvokeOptions{
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
	require.Len(t, textCollected, 2)
}

func TestInvokerErrorCapturesStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho 'some error' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	_, err = inv.Invoke(context.Background(), "test", llm.InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "claude exited")
	require.Contains(t, err.Error(), "some error")
}

func TestInvokerErrorWithoutStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\nexit 1\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	_, err = inv.Invoke(context.Background(), "test", llm.InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "claude exited")
	require.NotContains(t, err.Error(), "stderr")
}

func TestInvokerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\nsleep 30\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{Timeout: 1})
	require.NoError(t, err)
	require.Contains(t, res.Text, protocol.StatusBlockKey)
	require.Contains(t, res.Text, string(protocol.StatusBlocked))
}

func TestInvokerToolUseCallback(t *testing.T) {
	tmpDir := t.TempDir()
	script := `#!/bin/sh
cat >/dev/null
echo '{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","id":"t1","input":{"file_path":"/tmp/test.go"}}],"usage":{}}}'
echo '{"type":"user","tool_name":"Read","tool_result":"file contents here"}'
echo '{"type":"result","result":""}'
`
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})

	var toolName string
	var toolInput any
	var toolResultName, toolResultContent string
	opts := llm.InvokeOptions{
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

func TestBuildEnv(t *testing.T) {
	tests := []struct {
		name       string
		setEnv     map[string]string
		config     Config
		wantSet    map[string]string
		wantAbsent []string
	}{
		{
			name:       "filters inherited ANTHROPIC_API_KEY",
			setEnv:     map[string]string{"ANTHROPIC_API_KEY": "secret-inherited"},
			config:     Config{},
			wantAbsent: []string{"ANTHROPIC_API_KEY="},
		},
		{
			name:    "sets explicit ANTHROPIC_API_KEY",
			setEnv:  map[string]string{"ANTHROPIC_API_KEY": "should-be-filtered"},
			config:  Config{AnthropicAPIKey: "explicit-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "explicit-key"},
		},
		{
			name:    "sets CLAUDE_CONFIG_DIR from config",
			config:  Config{ClaudeConfigDir: "/custom/config"},
			wantSet: map[string]string{"CLAUDE_CONFIG_DIR": "/custom/config"},
		},
		{
			name:       "filters inherited CLAUDE_CONFIG_DIR",
			setEnv:     map[string]string{"CLAUDE_CONFIG_DIR": "/inherited"},
			config:     Config{},
			wantAbsent: []string{"CLAUDE_CONFIG_DIR="},
		},
		{
			name:   "empty config returns non-nil env",
			config: Config{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.setEnv {
				t.Setenv(k, v)
			}

			env := BuildEnv(tc.config)
			require.NotNil(t, env)

			for key, val := range tc.wantSet {
				expected := key + "=" + val
				require.True(t, slices.Contains(env, expected),
					"%s should be set in env", expected)
			}

			for _, prefix := range tc.wantAbsent {
				for _, e := range env {
					require.False(t, strings.HasPrefix(e, prefix),
						"%s should not be in env", prefix)
				}
			}
		})
	}
}
