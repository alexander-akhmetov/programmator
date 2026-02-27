package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCmdDefinition(t *testing.T) {
	assert.Equal(t, "run [prompt]", runCmd.Use)
	assert.NotEmpty(t, runCmd.Short)
	assert.NotEmpty(t, runCmd.Long)
}

func TestRunCmdFlags(t *testing.T) {
	flags := runCmd.Flags()

	dirFlag := flags.Lookup("dir")
	require.NotNil(t, dirFlag)
	assert.Equal(t, "d", dirFlag.Shorthand)

	printFlag := flags.Lookup("print")
	require.NotNil(t, printFlag)
	assert.Equal(t, "false", printFlag.DefValue)

	maxTurnsFlag := flags.Lookup("max-turns")
	require.NotNil(t, maxTurnsFlag)
	assert.Equal(t, "0", maxTurnsFlag.DefValue)
}

func TestBuildRunPrompt(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		stdin   string
		want    string
		wantErr string
	}{
		{
			name:    "no args and no stdin returns error",
			args:    []string{},
			wantErr: "no prompt provided",
		},
		{
			name: "single arg",
			args: []string{"hello"},
			want: "hello",
		},
		{
			name: "multiple args joined with spaces",
			args: []string{"hello", "world"},
			want: "hello world",
		},
		{
			name:  "stdin is used when no args",
			args:  []string{},
			stdin: "prompt from stdin\n",
			want:  "prompt from stdin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdin *os.File
			if tt.stdin != "" {
				f, err := os.CreateTemp(t.TempDir(), "stdin")
				require.NoError(t, err)
				_, err = f.WriteString(tt.stdin)
				require.NoError(t, err)
				_, err = f.Seek(0, 0)
				require.NoError(t, err)
				stdin = f
				defer f.Close()
			} else {
				f, err := os.Open(os.DevNull)
				require.NoError(t, err)
				stdin = f
				defer f.Close()
			}

			got, err := buildRunPrompt(tt.args, stdin)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// NOTE: Do not add t.Parallel() - this test mutates package-level variables.
func TestRunRunNoPrompt(t *testing.T) {
	err := runRun(nil, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no prompt provided")
}

func TestRunCmdHelp(t *testing.T) {
	assert.Contains(t, runCmd.Long, "Run the configured coding agent with a custom prompt")
	assert.Contains(t, runCmd.Long, "programmator run")
}

func TestBuildRunPromptArgsOverrideStdin(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	require.NoError(t, err)
	defer f.Close()
	_, _ = f.WriteString("stdin content")
	_, _ = f.Seek(0, 0)

	got, err := buildRunPrompt([]string{"from", "args"}, f)
	assert.NoError(t, err)
	assert.Equal(t, "from args", got)
	assert.NotContains(t, got, "stdin")
}

// NOTE: Do not add t.Parallel() - this test mutates package-level variables.
func TestBuildCommonFlags(t *testing.T) {
	origTurns := runMaxTurns
	defer func() {
		runMaxTurns = origTurns
	}()

	runMaxTurns = 0
	assert.Empty(t, buildCommonFlags())

	runMaxTurns = 10
	flags := buildCommonFlags()
	assert.Equal(t, []string{"--max-turns", "10"}, flags)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func TestStreamOutput(t *testing.T) {
	t.Run("normal input", func(_ *testing.T) {
		r := strings.NewReader("line1\nline2\n")
		streamOutput(r)
	})

	t.Run("reader error", func(_ *testing.T) {
		streamOutput(errReader{})
	})

	t.Run("empty input", func(_ *testing.T) {
		streamOutput(io.LimitReader(strings.NewReader(""), 0))
	})
}
