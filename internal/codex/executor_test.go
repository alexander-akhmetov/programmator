package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner implements Runner for testing.
type mockRunner struct {
	runFunc func(ctx context.Context, name string, args ...string) (Streams, func() error, error)
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) (Streams, func() error, error) {
	return m.runFunc(ctx, name, args...)
}

func mockStreams(stderr, stdout string) Streams {
	return Streams{
		Stderr: strings.NewReader(stderr),
		Stdout: strings.NewReader(stdout),
	}
}

func mockWait() func() error {
	return func() error { return nil }
}

func mockWaitError(err error) func() error {
	return func() error { return err }
}

func TestExecutor_Run_Success(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			stderr := "--------\nmodel: gpt-5\n--------\n**Analyzing...**\n"
			stdout := "Analysis complete: no issues found.\n<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>"
			return mockStreams(stderr, stdout), mockWait(), nil
		},
	}
	e := &Executor{runner: mock}

	result := e.Run(context.Background(), "analyze code")

	require.NoError(t, result.Error)
	assert.Contains(t, result.Output, "Analysis complete")
	assert.Equal(t, "<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>", result.Signal)
}

func TestExecutor_Run_StreamsStderr(t *testing.T) {
	stderr := `--------
OpenAI Codex v1.2.3
model: gpt-5
workdir: /tmp/test
sandbox: read-only
--------
Some thinking noise
**Summary: Found 2 issues**
More thinking
**Details: processing...**
Even more noise`

	stdout := `Final response from codex.
<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>`

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return mockStreams(stderr, stdout), mockWait(), nil
		},
	}

	var streamedLines []string
	e := &Executor{
		runner:        mock,
		OutputHandler: func(text string) { streamedLines = append(streamedLines, strings.TrimSuffix(text, "\n")) },
	}

	result := e.Run(context.Background(), "analyze code")

	require.NoError(t, result.Error)

	// header block is shown (between first two "--------" separators)
	assert.Contains(t, streamedLines, "--------")
	assert.Contains(t, streamedLines, "OpenAI Codex v1.2.3")
	assert.Contains(t, streamedLines, "model: gpt-5")
	assert.Contains(t, streamedLines, "sandbox: read-only")

	// bold summaries are shown (stripped of ** markers)
	assert.Contains(t, streamedLines, "Summary: Found 2 issues")
	assert.Contains(t, streamedLines, "Details: processing...")

	// noise is filtered
	for _, line := range streamedLines {
		assert.NotContains(t, line, "Some thinking noise")
		assert.NotContains(t, line, "More thinking")
		assert.NotContains(t, line, "Even more noise")
	}

	// Result.Output contains stdout (the actual response)
	assert.Contains(t, result.Output, "Final response from codex")
	assert.Equal(t, "<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>", result.Signal)
}

func TestExecutor_Run_StdoutIsResult(t *testing.T) {
	stderr := "--------\nheader\n--------\n**progress**\nthinking noise\n"
	stdout := "This is the actual answer from codex."

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return mockStreams(stderr, stdout), mockWait(), nil
		},
	}

	e := &Executor{runner: mock}
	result := e.Run(context.Background(), "analyze code")

	require.NoError(t, result.Error)
	assert.Equal(t, stdout, result.Output)
	assert.NotContains(t, result.Output, "progress")
	assert.NotContains(t, result.Output, "thinking noise")
}

func TestExecutor_Run_StartError(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return Streams{}, nil, errors.New("command not found")
		},
	}
	e := &Executor{runner: mock}

	result := e.Run(context.Background(), "analyze code")

	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "start codex")
	assert.Contains(t, result.Error.Error(), "command not found")
}

func TestExecutor_Run_WaitError(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return mockStreams("", "partial output"), mockWaitError(errors.New("exit 1")), nil
		},
	}
	e := &Executor{runner: mock}

	result := e.Run(context.Background(), "analyze code")

	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "codex exited with error")
	assert.Equal(t, "partial output", result.Output)
}

func TestExecutor_Run_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return mockStreams("", ""), mockWaitError(context.Canceled), nil
		},
	}
	e := &Executor{runner: mock}

	result := e.Run(ctx, "analyze code")

	require.ErrorIs(t, result.Error, context.Canceled)
}

func TestExecutor_Run_DefaultSettings(t *testing.T) {
	var capturedArgs []string
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, args ...string) (Streams, func() error, error) {
			capturedArgs = args
			return mockStreams("", "result"), mockWait(), nil
		},
	}
	e := &Executor{runner: mock}

	result := e.Run(context.Background(), "test prompt")

	require.NoError(t, result.Error)

	argsStr := strings.Join(capturedArgs, " ")
	assert.Contains(t, argsStr, `model="gpt-5.2-codex"`)
	assert.Contains(t, argsStr, "model_reasoning_effort=xhigh")
	assert.Contains(t, argsStr, "stream_idle_timeout_ms=3600000")
	assert.Contains(t, argsStr, "--sandbox read-only")
}

func TestExecutor_Run_CustomSettings(t *testing.T) {
	var capturedCmd string
	var capturedArgs []string
	mock := &mockRunner{
		runFunc: func(_ context.Context, name string, args ...string) (Streams, func() error, error) {
			capturedCmd = name
			capturedArgs = args
			return mockStreams("", "result"), mockWait(), nil
		},
	}
	e := &Executor{
		runner:          mock,
		Command:         "custom-codex",
		Model:           "gpt-4o",
		ReasoningEffort: "medium",
		TimeoutMs:       1000,
		Sandbox:         "off",
		ProjectDoc:      "/path/to/doc.md",
	}

	result := e.Run(context.Background(), "test")

	require.NoError(t, result.Error)
	assert.Equal(t, "custom-codex", capturedCmd)

	assert.Equal(t, "exec", capturedArgs[0])
	assert.True(t, slices.Contains(capturedArgs, `model="gpt-4o"`), "expected model setting in args: %v", capturedArgs)

	argsStr := strings.Join(capturedArgs, " ")
	assert.Contains(t, argsStr, "model_reasoning_effort=medium")
	assert.Contains(t, argsStr, "stream_idle_timeout_ms=1000")
	assert.Contains(t, argsStr, "--sandbox off")
	assert.Contains(t, argsStr, `project_doc="/path/to/doc.md"`)
}

func TestExecutor_Run_InvalidModelName(t *testing.T) {
	tests := []struct {
		name  string
		model string
	}{
		{"shell injection", "model; rm -rf /"},
		{"path traversal", "model:/etc/passwd"},
		{"with slash", "org/model"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &Executor{Model: tc.model}

			result := e.Run(context.Background(), "test")

			require.Error(t, result.Error)
			assert.Contains(t, result.Error.Error(), "invalid model name")
		})
	}
}

func TestExecutor_shouldDisplay_headerBlock(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		name       string
		lines      []string
		wantShown  []string
		wantHidden []string
	}{
		{
			name: "header block between separators",
			lines: []string{
				"--------",
				"OpenAI Codex v1.2.3",
				"model: gpt-5",
				"workdir: /tmp/test",
				"--------",
				"noise after header",
			},
			wantShown:  []string{"--------", "OpenAI Codex v1.2.3", "model: gpt-5", "workdir: /tmp/test", "--------"},
			wantHidden: []string{"noise after header"},
		},
		{
			name: "third separator not shown",
			lines: []string{
				"--------",
				"header content",
				"--------",
				"--------",
				"more content",
			},
			wantShown:  []string{"--------", "header content", "--------"},
			wantHidden: []string{"more content"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &filterState{}
			var shown []string
			for _, line := range tc.lines {
				if ok, out := e.shouldDisplay(line, state); ok {
					shown = append(shown, out)
				}
			}
			for _, want := range tc.wantShown {
				assert.Contains(t, shown, want, "expected shown: %s", want)
			}
			for _, notWant := range tc.wantHidden {
				assert.NotContains(t, shown, notWant, "expected hidden: %s", notWant)
			}
			wantSepCount, gotSepCount := 0, 0
			for _, s := range tc.wantShown {
				if s == "--------" {
					wantSepCount++
				}
			}
			for _, s := range shown {
				if s == "--------" {
					gotSepCount++
				}
			}
			assert.Equal(t, wantSepCount, gotSepCount, "separator count mismatch")
		})
	}
}

func TestExecutor_shouldDisplay_boldSummaries(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		name    string
		line    string
		state   *filterState
		wantOk  bool
		wantOut string
	}{
		{
			name:    "bold shown after header",
			line:    "**Summary: Found issues**",
			state:   &filterState{headerCount: 2},
			wantOk:  true,
			wantOut: "Summary: Found issues",
		},
		{
			name:    "bold shown before header ends",
			line:    "**Progress...**",
			state:   &filterState{headerCount: 0},
			wantOk:  true,
			wantOut: "Progress...",
		},
		{
			name:    "non-bold filtered after header",
			line:    "Some random noise",
			state:   &filterState{headerCount: 2},
			wantOk:  false,
			wantOut: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, out := e.shouldDisplay(tc.line, tc.state)
			assert.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.wantOut, out)
		})
	}
}

func TestExecutor_shouldDisplay_emptyAndWhitespace(t *testing.T) {
	e := &Executor{}
	state := &filterState{headerCount: 1}

	tests := []struct {
		line   string
		wantOk bool
	}{
		{"", false},
		{"   ", false},
		{"\t", false},
		{"\n", false},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("whitespace_case_%d", i), func(t *testing.T) {
			ok, _ := e.shouldDisplay(tc.line, state)
			assert.Equal(t, tc.wantOk, ok)
		})
	}
}

func TestStripBold(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no bold", "plain text", "plain text"},
		{"single bold", "**bold** text", "bold text"},
		{"multiple bold", "**one** and **two**", "one and two"},
		{"nested in text", "before **middle** after", "before middle after"},
		{"unclosed bold", "**unclosed text", "**unclosed text"},
		{"empty bold", "**** empty", " empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripBold(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExecutor_Run_NoOutputHandler(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return mockStreams("**progress**", "actual output"), mockWait(), nil
		},
	}

	e := &Executor{runner: mock, OutputHandler: nil}
	result := e.Run(context.Background(), "analyze code")

	require.NoError(t, result.Error)
	assert.Equal(t, "actual output", result.Output)
}

func TestExecutor_processStderr_contextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pr, pw := io.Pipe()

	go func() {
		_, _ = pw.Write([]byte("--------\n"))
		_, _ = pw.Write([]byte("header line\n"))
		_, _ = pw.Write([]byte("--------\n"))
		cancel()
		// Write more data after cancel to ensure the scanner loop checks ctx
		for range 100 {
			_, _ = pw.Write([]byte("**after cancel**\n"))
		}
		pw.Close()
	}()

	e := &Executor{}
	err := e.processStderr(ctx, pr)

	// After cancellation, processStderr should return a context error
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExecRunner_Run(t *testing.T) {
	runner := &execRunner{}

	streams, wait, err := runner.Run(context.Background(), "echo", "hello")

	require.NoError(t, err)
	require.NotNil(t, streams.Stdout)
	require.NotNil(t, streams.Stderr)
	require.NotNil(t, wait)

	data, readErr := io.ReadAll(streams.Stdout)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "hello")

	err = wait()
	require.NoError(t, err)
}

func TestExecRunner_Run_ContextAlreadyCanceled(t *testing.T) {
	runner := &execRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := runner.Run(ctx, "echo", "hello")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context already canceled")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExecRunner_Run_CommandNotFound(t *testing.T) {
	runner := &execRunner{}

	_, _, err := runner.Run(context.Background(), "nonexistent-command-12345")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "start command")
}

func TestExecutor_readStdout(t *testing.T) {
	e := &Executor{}

	content := "This is the stdout content\nWith multiple lines\n"
	result, err := e.readStdout(strings.NewReader(content))

	require.NoError(t, err)
	assert.Equal(t, content, result)
}

// failingReader is a reader that always returns an error.
type failingReader struct {
	err error
}

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func TestExecutor_processStderr_readError(t *testing.T) {
	e := &Executor{}
	errReader := &failingReader{err: errors.New("read failed")}

	err := e.processStderr(context.Background(), errReader)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read stderr")
}

func TestExecutor_readStdout_error(t *testing.T) {
	e := &Executor{}
	errReader := &failingReader{err: errors.New("read failed")}

	_, err := e.readStdout(errReader)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read stdout")
}

func TestExecutor_Run_ErrorPriority(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			return Streams{
				Stderr: &failingReader{err: errors.New("stderr failed")},
				Stdout: strings.NewReader("output"),
			}, mockWaitError(errors.New("wait failed")), nil
		},
	}
	e := &Executor{runner: mock}

	result := e.Run(context.Background(), "test")

	require.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "stderr")
}

func TestExecutor_shouldDisplay_deduplication(t *testing.T) {
	e := &Executor{}

	tests := []struct {
		name      string
		lines     []string
		wantShown []string
	}{
		{
			name: "duplicate bold summaries filtered",
			lines: []string{
				"--------",
				"header",
				"--------",
				"**Findings**",
				"**Questions**",
				"**Change Summary**",
				"**Findings**",
				"**Questions**",
				"**Change Summary**",
			},
			wantShown: []string{"--------", "header", "Findings", "Questions", "Change Summary"},
		},
		{
			name: "non-consecutive duplicates filtered",
			lines: []string{
				"--------",
				"model: gpt-5",
				"--------",
				"**Processing...**",
				"**Done**",
				"**Processing...**",
			},
			wantShown: []string{"--------", "model: gpt-5", "Processing...", "Done"},
		},
		{
			name: "separators not deduplicated",
			lines: []string{
				"--------",
				"header content",
				"--------",
				"--------",
			},
			wantShown: []string{"--------", "header content", "--------"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &filterState{}
			var shown []string
			for _, line := range tc.lines {
				if ok, out := e.shouldDisplay(line, state); ok {
					shown = append(shown, out)
				}
			}

			for _, want := range tc.wantShown {
				assert.True(t, slices.Contains(shown, want), "expected %q to appear in shown output", want)
			}

			// no duplicates in shown output (except separators which can appear twice)
			seenLines := make(map[string]int)
			for _, s := range shown {
				seenLines[s]++
			}
			for line, count := range seenLines {
				if line == "--------" {
					assert.LessOrEqual(t, count, 2)
				} else {
					assert.Equal(t, 1, count, "duplicate line found: %q", line)
				}
			}
		})
	}
}

func TestExecutor_processStderr_largeLines(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"100KB line", 100 * 1024},
		{"500KB line", 500 * 1024},
		{"1MB line", 1024 * 1024},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			largeContent := strings.Repeat("x", tc.size)
			stderr := "--------\n" + largeContent + "\n--------\n"

			var shown []string
			e := &Executor{
				OutputHandler: func(text string) {
					shown = append(shown, strings.TrimSuffix(text, "\n"))
				},
			}

			err := e.processStderr(context.Background(), strings.NewReader(stderr))

			require.NoError(t, err)
			assert.Contains(t, shown, largeContent)
		})
	}
}

func TestExecutor_Run_largeOutput(t *testing.T) {
	largeStderr := strings.Repeat("x", 200*1024)
	largeStdout := strings.Repeat("y", 500*1024)

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			stderr := "--------\n" + largeStderr + "\n--------\n"
			return mockStreams(stderr, largeStdout), mockWait(), nil
		},
	}

	var captured []string
	e := &Executor{
		runner:        mock,
		OutputHandler: func(text string) { captured = append(captured, text) },
	}

	result := e.Run(context.Background(), "test")

	require.NoError(t, result.Error)
	assert.Equal(t, largeStdout, result.Output)
	found := false
	for _, c := range captured {
		if strings.Contains(c, strings.Repeat("x", 100)) {
			found = true
			break
		}
	}
	assert.True(t, found, "large stderr content should be captured")
}

func TestExecutor_Run_ErrorPattern(t *testing.T) {
	tests := []struct {
		name        string
		stdout      string
		patterns    []string
		wantError   bool
		wantPattern string
		wantOutput  string
	}{
		{
			name:       "no patterns configured",
			stdout:     "Rate limit exceeded",
			patterns:   nil,
			wantError:  false,
			wantOutput: "Rate limit exceeded",
		},
		{
			name:       "pattern not matched",
			stdout:     "Analysis complete: no issues found",
			patterns:   []string{"rate limit", "quota exceeded"},
			wantError:  false,
			wantOutput: "Analysis complete: no issues found",
		},
		{
			name:        "pattern matched",
			stdout:      "Error: Rate limit exceeded, please try again later",
			patterns:    []string{"rate limit"},
			wantError:   true,
			wantPattern: "rate limit",
			wantOutput:  "Error: Rate limit exceeded, please try again later",
		},
		{
			name:        "case insensitive match",
			stdout:      "QUOTA EXCEEDED for your account",
			patterns:    []string{"quota exceeded"},
			wantError:   true,
			wantPattern: "quota exceeded",
			wantOutput:  "QUOTA EXCEEDED for your account",
		},
		{
			name:        "first matching pattern returned",
			stdout:      "rate limit and quota exceeded",
			patterns:    []string{"rate limit", "quota exceeded"},
			wantError:   true,
			wantPattern: "rate limit",
			wantOutput:  "rate limit and quota exceeded",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockRunner{
				runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
					return mockStreams("", tc.stdout), mockWait(), nil
				},
			}
			e := &Executor{
				runner:        mock,
				ErrorPatterns: tc.patterns,
			}

			result := e.Run(context.Background(), "analyze code")

			assert.Equal(t, tc.wantOutput, result.Output)

			if tc.wantError {
				require.Error(t, result.Error)
				var patternErr *PatternMatchError
				require.ErrorAs(t, result.Error, &patternErr)
				assert.Equal(t, tc.wantPattern, patternErr.Pattern)
				assert.Equal(t, "codex /status", patternErr.HelpCmd)
			} else {
				require.NoError(t, result.Error)
			}
		})
	}
}

func TestExecutor_Run_ErrorPattern_WithSignal(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (Streams, func() error, error) {
			stdout := "Rate limit exceeded <<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>"
			return mockStreams("", stdout), mockWait(), nil
		},
	}
	e := &Executor{
		runner:        mock,
		ErrorPatterns: []string{"rate limit"},
	}

	result := e.Run(context.Background(), "analyze code")

	require.Error(t, result.Error)
	var patternErr *PatternMatchError
	require.ErrorAs(t, result.Error, &patternErr)
	assert.Equal(t, "rate limit", patternErr.Pattern)
	assert.Equal(t, "codex /status", patternErr.HelpCmd)

	assert.Contains(t, result.Output, "Rate limit exceeded")
	assert.Equal(t, "<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>", result.Signal)
}

func TestDetectSignal(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"no signal", "regular output", ""},
		{"codex review done", "output <<<PROGRAMMATOR:CODEX_REVIEW_DONE>>> more", "<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>"},
		{"partial signal", "<<<PROGRAMMATOR:UNKNOWN>>>", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectSignal(tc.text)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckErrorPatterns(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		patterns []string
		want     string
	}{
		{"nil patterns", "output", nil, ""},
		{"empty patterns", "output", []string{}, ""},
		{"no match", "output", []string{"error"}, ""},
		{"match", "Rate limit hit", []string{"rate limit"}, "rate limit"},
		{"whitespace pattern", "output", []string{"  "}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkErrorPatterns(tc.output, tc.patterns)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDetectBinary(t *testing.T) {
	// echo should always exist
	path, found := DetectBinary("echo")
	assert.True(t, found)
	assert.NotEmpty(t, path)

	// nonexistent binary
	_, found = DetectBinary("nonexistent-binary-xyz-12345")
	assert.False(t, found)
}

func TestAvailable(t *testing.T) {
	result := Available()
	// Verify the result is consistent with DetectBinary
	_, found := DetectBinary("codex")
	assert.Equal(t, found, result)
}
