package pi

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
			name:       "filters all provider API keys",
			setEnv:     map[string]string{"ANTHROPIC_API_KEY": "a", "OPENAI_API_KEY": "b", "GEMINI_API_KEY": "c"},
			config:     Config{},
			wantAbsent: []string{"ANTHROPIC_API_KEY=", "OPENAI_API_KEY=", "GEMINI_API_KEY="},
		},
		{
			name:    "anthropic provider sets ANTHROPIC_API_KEY",
			setEnv:  map[string]string{"ANTHROPIC_API_KEY": "should-be-filtered"},
			config:  Config{Provider: "anthropic", APIKey: "explicit-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "explicit-key"},
		},
		{
			name:       "openai provider sets OPENAI_API_KEY",
			config:     Config{Provider: "openai", APIKey: "sk-openai"},
			wantSet:    map[string]string{"OPENAI_API_KEY": "sk-openai"},
			wantAbsent: []string{"ANTHROPIC_API_KEY="},
		},
		{
			name:    "empty provider defaults to ANTHROPIC_API_KEY",
			config:  Config{APIKey: "fallback-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "fallback-key"},
		},
		{
			name:    "sets PI_CODING_AGENT_DIR",
			config:  Config{ConfigDir: "/custom/pi/config"},
			wantSet: map[string]string{"PI_CODING_AGENT_DIR": "/custom/pi/config"},
		},
		{
			name:       "filters inherited PI_CODING_AGENT_DIR",
			setEnv:     map[string]string{"PI_CODING_AGENT_DIR": "/old/dir"},
			config:     Config{ConfigDir: "/new/dir"},
			wantSet:    map[string]string{"PI_CODING_AGENT_DIR": "/new/dir"},
			wantAbsent: []string{"PI_CODING_AGENT_DIR=/old/dir"},
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

func TestInvokerTextMode(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
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
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
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
echo '{"type":"session","version":3,"id":"test","timestamp":"2025-01-01T00:00:00Z","cwd":"/tmp"}'
echo '{"type":"message_start","message":{"role":"assistant","model":"test-model","content":[]}}'
echo '{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"text_delta","delta":"Hello"}}'
echo '{"type":"message_update","message":{"role":"assistant"},"assistantMessageEvent":{"type":"text_delta","delta":" World"}}'
echo '{"type":"message_end","message":{"role":"assistant","usage":{"input":20,"output":13}}}'
echo '{"type":"agent_end","messages":[{"role":"assistant","model":"test-model","usage":{"input":20,"output":13}}]}'
`
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
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
	script := "#!/bin/sh\ncat >/dev/null\necho 'some error' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	_, err = inv.Invoke(context.Background(), "test", llm.InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pi exited")
	require.Contains(t, err.Error(), "some error")
}

func TestInvokerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\nsleep 30\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{Timeout: 1})
	require.NoError(t, err)
	require.Contains(t, res.Text, protocol.StatusBlockKey)
	require.Contains(t, res.Text, string(protocol.StatusBlocked))
}

func TestInvokerProviderAndModelFlags(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\"\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{
		Provider: "anthropic",
		Model:    "sonnet",
	})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{})
	require.NoError(t, err)
	require.Contains(t, res.Text, "--provider anthropic")
	require.Contains(t, res.Text, "--model sonnet")
	require.Contains(t, res.Text, "--print")
}
