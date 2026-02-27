package llm

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestBuildPiEnv(t *testing.T) {
	tests := []struct {
		name       string
		setEnv     map[string]string
		config     PiEnvConfig
		wantSet    map[string]string
		wantAbsent []string
	}{
		{
			name:       "filters inherited ANTHROPIC_API_KEY",
			setEnv:     map[string]string{"ANTHROPIC_API_KEY": "secret-inherited"},
			config:     PiEnvConfig{},
			wantAbsent: []string{"ANTHROPIC_API_KEY="},
		},
		{
			name:    "sets explicit API key",
			setEnv:  map[string]string{"ANTHROPIC_API_KEY": "should-be-filtered"},
			config:  PiEnvConfig{APIKey: "explicit-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "explicit-key"},
		},
		{
			name:    "sets PI_CODING_AGENT_DIR",
			config:  PiEnvConfig{ConfigDir: "/custom/pi/config"},
			wantSet: map[string]string{"PI_CODING_AGENT_DIR": "/custom/pi/config"},
		},
		{
			name:   "empty config returns non-nil env",
			config: PiEnvConfig{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.setEnv {
				t.Setenv(k, v)
			}

			env := BuildPiEnv(tc.config)
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

func TestPiInvokerTextMode(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewPiInvoker(PiEnvConfig{})
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

func TestPiInvokerWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\npwd\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	workDir := t.TempDir()
	inv := NewPiInvoker(PiEnvConfig{})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{WorkingDir: workDir})
	require.NoError(t, err)
	require.Contains(t, res.Text, workDir)
}

func TestPiInvokerStreamingMode(t *testing.T) {
	tmpDir := t.TempDir()
	// Fake pi that outputs JSON events. In streaming mode prompt is a positional arg,
	// so the script ignores it and just outputs events.
	script := `#!/bin/sh
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

	inv := NewPiInvoker(PiEnvConfig{})

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
	require.Len(t, textCollected, 2)
}

func TestPiInvokerErrorCapturesStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho 'some error' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewPiInvoker(PiEnvConfig{})
	_, err = inv.Invoke(context.Background(), "test", InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pi exited")
	require.Contains(t, err.Error(), "some error")
}

func TestPiInvokerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\nsleep 30\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewPiInvoker(PiEnvConfig{})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{Timeout: 1})
	require.NoError(t, err)
	require.Contains(t, res.Text, protocol.StatusBlockKey)
	require.Contains(t, res.Text, string(protocol.StatusBlocked))
}

func TestPiInvokerProviderAndModelFlags(t *testing.T) {
	tmpDir := t.TempDir()
	// Script that prints all arguments to verify flags are passed correctly
	script := "#!/bin/sh\necho \"$@\"\n"
	err := os.WriteFile(tmpDir+"/pi", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewPiInvoker(PiEnvConfig{
		Provider: "anthropic",
		Model:    "sonnet",
	})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{})
	require.NoError(t, err)
	require.Contains(t, res.Text, "--provider anthropic")
	require.Contains(t, res.Text, "--model sonnet")
	require.Contains(t, res.Text, "--print")
}
