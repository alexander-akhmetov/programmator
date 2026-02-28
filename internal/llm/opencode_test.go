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

func TestBuildOpenCodeEnv(t *testing.T) {
	tests := []struct {
		name       string
		setEnv     map[string]string
		config     OpenCodeEnvConfig
		wantSet    map[string]string
		wantAbsent []string
	}{
		{
			name:       "filters inherited OPENCODE_CONFIG_DIR",
			setEnv:     map[string]string{"OPENCODE_CONFIG_DIR": "/old/dir"},
			config:     OpenCodeEnvConfig{ConfigDir: "/new/dir"},
			wantSet:    map[string]string{"OPENCODE_CONFIG_DIR": "/new/dir"},
			wantAbsent: []string{"OPENCODE_CONFIG_DIR=/old/dir"},
		},
		{
			name:    "sets OPENCODE_CONFIG_DIR from config",
			config:  OpenCodeEnvConfig{ConfigDir: "/custom/opencode"},
			wantSet: map[string]string{"OPENCODE_CONFIG_DIR": "/custom/opencode"},
		},
		{
			name:       "omits OPENCODE_CONFIG_DIR when config is empty",
			config:     OpenCodeEnvConfig{},
			wantAbsent: []string{"OPENCODE_CONFIG_DIR="},
		},
		{
			name:    "anthropic model prefix sets ANTHROPIC_API_KEY",
			config:  OpenCodeEnvConfig{Model: "anthropic/claude-sonnet-4-5", APIKey: "sk-ant-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "sk-ant-key"},
		},
		{
			name:    "openai model prefix sets OPENAI_API_KEY",
			config:  OpenCodeEnvConfig{Model: "openai/gpt-4o", APIKey: "sk-openai-key"},
			wantSet: map[string]string{"OPENAI_API_KEY": "sk-openai-key"},
		},
		{
			name:    "google model prefix sets GEMINI_API_KEY",
			config:  OpenCodeEnvConfig{Model: "google/gemini-pro", APIKey: "gemini-key"},
			wantSet: map[string]string{"GEMINI_API_KEY": "gemini-key"},
		},
		{
			name:    "unknown provider prefix falls back to ANTHROPIC_API_KEY",
			config:  OpenCodeEnvConfig{Model: "custom-provider/some-model", APIKey: "fallback-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "fallback-key"},
		},
		{
			name:       "no API key set means no key env var in output",
			config:     OpenCodeEnvConfig{Model: "anthropic/claude-sonnet-4-5"},
			wantAbsent: []string{"ANTHROPIC_API_KEY=", "OPENAI_API_KEY=", "GEMINI_API_KEY="},
		},
		{
			name:   "empty config returns non-nil env",
			config: OpenCodeEnvConfig{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.setEnv {
				t.Setenv(k, v)
			}

			env := BuildOpenCodeEnv(tc.config)
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

func TestProviderFromModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "anthropic prefix", model: "anthropic/claude-sonnet-4-5", want: "anthropic"},
		{name: "openai prefix", model: "openai/gpt-4o", want: "openai"},
		{name: "google prefix", model: "google/gemini-pro", want: "google"},
		{name: "no slash", model: "no-slash", want: ""},
		{name: "empty model", model: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := providerFromModel(tc.model)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestOpenCodeInvokerTextMode(t *testing.T) {
	tmpDir := t.TempDir()
	// OpenCode receives the prompt as the last positional argument, not stdin.
	// This script echoes the last positional argument.
	script := "#!/bin/sh\nfor last; do true; done\necho \"$last\"\n"
	err := os.WriteFile(tmpDir+"/opencode", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewOpenCodeInvoker(OpenCodeEnvConfig{})
	var collected []string
	opts := InvokeOptions{
		OnOutput: func(text string) {
			collected = append(collected, text)
		},
	}

	res, err := inv.Invoke(context.Background(), "hello world", opts)
	require.NoError(t, err)
	// The script echoes the last positional arg which is the prompt
	require.Contains(t, res.Text, "hello world")
	require.NotEmpty(t, collected)
}

func TestOpenCodeInvokerStreamingMode(t *testing.T) {
	tmpDir := t.TempDir()
	// Fake opencode that outputs nd-JSON events when --format json is passed.
	script := `#!/bin/sh
echo '{"type":"step_start","part":{"type":"step-start","snapshot":"abc123"}}'
echo '{"type":"text","part":{"type":"text","text":"Hello World"}}'
echo '{"type":"step_finish","part":{"type":"step-finish","reason":"end_turn","cost":0.01,"tokens":{"total":33,"input":20,"output":13,"reasoning":0,"cache":{"read":5,"write":3}}}}'
`
	err := os.WriteFile(tmpDir+"/opencode", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewOpenCodeInvoker(OpenCodeEnvConfig{Model: "anthropic/claude-sonnet-4-5"})

	var model string
	var finalModel string
	var finalInput, finalOutput int
	var textCollected []string
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
	require.Equal(t, "anthropic/claude-sonnet-4-5", model)
	require.Equal(t, "anthropic/claude-sonnet-4-5", finalModel)
	require.Equal(t, 28, finalInput)  // 20 + 5 + 3
	require.Equal(t, 13, finalOutput) // 13
	require.Len(t, textCollected, 1)
	require.Equal(t, "Hello World", textCollected[0])
}

func TestOpenCodeInvokerErrorCapturesStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho 'some error' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/opencode", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewOpenCodeInvoker(OpenCodeEnvConfig{})
	_, err = inv.Invoke(context.Background(), "test", InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "opencode exited")
	require.Contains(t, err.Error(), "some error")
}

func TestOpenCodeInvokerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\nsleep 30\n"
	err := os.WriteFile(tmpDir+"/opencode", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewOpenCodeInvoker(OpenCodeEnvConfig{})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{Timeout: 1})
	require.NoError(t, err)
	require.Contains(t, res.Text, protocol.StatusBlockKey)
	require.Contains(t, res.Text, string(protocol.StatusBlocked))
}

func TestOpenCodeInvokerModelAndQuietFlags(t *testing.T) {
	tmpDir := t.TempDir()
	// Script that prints all arguments to verify flags are passed correctly
	script := "#!/bin/sh\necho \"$@\"\n"
	err := os.WriteFile(tmpDir+"/opencode", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := NewOpenCodeInvoker(OpenCodeEnvConfig{
		Model: "anthropic/claude-sonnet-4-5",
	})
	res, err := inv.Invoke(context.Background(), "test", InvokeOptions{})
	require.NoError(t, err)
	require.Contains(t, res.Text, "--model anthropic/claude-sonnet-4-5")
	require.Contains(t, res.Text, "-q")
	require.Contains(t, res.Text, "run")
}
