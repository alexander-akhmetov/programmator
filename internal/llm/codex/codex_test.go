package codex

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
			name:       "filters inherited OPENAI_API_KEY",
			setEnv:     map[string]string{"OPENAI_API_KEY": "old-key"},
			config:     Config{APIKey: "new-key"},
			wantSet:    map[string]string{"OPENAI_API_KEY": "new-key"},
			wantAbsent: []string{"OPENAI_API_KEY=old-key"},
		},
		{
			name:    "sets OPENAI_API_KEY from config",
			config:  Config{APIKey: "sk-test"},
			wantSet: map[string]string{"OPENAI_API_KEY": "sk-test"},
		},
		{
			name:       "omits OPENAI_API_KEY when config is empty",
			config:     Config{},
			wantAbsent: []string{"OPENAI_API_KEY="},
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
	script := "#!/bin/sh\nfor last; do true; done\necho \"$last\"\n"
	err := os.WriteFile(tmpDir+"/codex", []byte(script), 0o755)
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
	require.Contains(t, res.Text, "hello world")
	require.NotEmpty(t, collected)
}

func TestInvokerStreamingMode(t *testing.T) {
	tmpDir := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  *--json*)
    echo '{"type":"thread.started","thread_id":"t1"}'
    echo '{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"Hello World"}}'
    echo '{"type":"turn.completed","usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":13}}'
    ;;
  *) echo "plain text output" ;;
esac
`
	err := os.WriteFile(tmpDir+"/codex", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{Model: "o3"})

	var model string
	var finalModel string
	var finalInput, finalOutput int
	var textCollected []string
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
	require.Equal(t, "o3", model)
	require.Equal(t, "o3", finalModel)
	require.Equal(t, 25, finalInput) // 20 + 5
	require.Equal(t, 13, finalOutput)
	require.Len(t, textCollected, 1)
	require.Equal(t, "Hello World", textCollected[0])
}

func TestInvokerErrorCapturesStderr(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho 'some error' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/codex", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	_, err = inv.Invoke(context.Background(), "test", llm.InvokeOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "codex exited")
	require.Contains(t, err.Error(), "some error")
}

func TestInvokerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\nsleep 30\n"
	err := os.WriteFile(tmpDir+"/codex", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{Timeout: 1})
	require.NoError(t, err)
	require.Contains(t, res.Text, protocol.StatusBlockKey)
	require.Contains(t, res.Text, string(protocol.StatusBlocked))
}

func TestInvokerModelFlag(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\"\n"
	err := os.WriteFile(tmpDir+"/codex", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	inv := New(Config{Model: "o3"})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{})
	require.NoError(t, err)
	require.Contains(t, res.Text, "-m o3")
	require.Contains(t, res.Text, "exec")
}

func TestInvokerWorkingDir(t *testing.T) {
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\"\n"
	err := os.WriteFile(tmpDir+"/codex", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+os.Getenv("PATH"))

	workDir := t.TempDir()

	inv := New(Config{})
	res, err := inv.Invoke(context.Background(), "test", llm.InvokeOptions{
		WorkingDir: workDir,
	})
	require.NoError(t, err)
	require.Contains(t, res.Text, "--cd "+workDir)
}
