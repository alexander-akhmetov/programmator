// Package codex provides execution of Codex CLI commands with output filtering.
package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// maxScannerBuffer is the maximum buffer size for bufio.Scanner.
// Set to 64MB to handle large outputs (e.g., diffs of large JSON files).
const maxScannerBuffer = 64 * 1024 * 1024

// validModelName matches expected model name patterns (alphanumeric, dots, dashes, colons).
// Forward slashes are disallowed to prevent path-like model names.
var validModelName = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

// Result holds execution result with output and detected signal.
type Result struct {
	Output string // accumulated text output (stdout)
	Signal string // detected signal or empty
	Error  error  // execution error if any
}

// PatternMatchError is returned when a configured error pattern is detected in output.
type PatternMatchError struct {
	Pattern string // the pattern that matched
	HelpCmd string // command to run for more information
}

func (e *PatternMatchError) Error() string {
	return fmt.Sprintf("detected error pattern: %q", e.Pattern)
}

// Executor runs codex CLI commands and filters output.
type Executor struct {
	Command         string            // command to execute, defaults to "codex"
	Model           string            // model to use, defaults to gpt-5.2-codex
	ReasoningEffort string            // reasoning effort level, defaults to "xhigh"
	TimeoutMs       int               // stream idle timeout in ms, defaults to 3600000
	Sandbox         string            // sandbox mode, defaults to "read-only"
	ProjectDoc      string            // path to project documentation file
	OutputHandler   func(text string) // called for each filtered output line in real-time
	ErrorPatterns   []string          // patterns to detect in output (e.g., rate limit messages)
	WorkingDir      string            // working directory for the codex process; empty uses process cwd
	runner          Runner            // for testing, nil uses default
}

// filterState tracks header separator count for filtering.
type filterState struct {
	headerCount int             // tracks "--------" separators seen (show content between first two)
	seen        map[string]bool // track all shown lines for deduplication
}

// SetRunner sets the runner for testing purposes.
func (e *Executor) SetRunner(r Runner) {
	e.runner = r
}

// Run executes codex CLI with the given prompt and returns filtered output.
// stderr is streamed line-by-line to OutputHandler for progress indication.
// stdout is captured entirely as the final response (returned in Result.Output).
func (e *Executor) Run(ctx context.Context, prompt string) Result {
	cmd := e.Command
	if cmd == "" {
		cmd = "codex"
	}

	model := e.Model
	if model == "" {
		model = "gpt-5.2-codex"
	}
	if !validModelName.MatchString(model) {
		return Result{Error: fmt.Errorf("invalid model name: %q", model)}
	}

	reasoningEffort := e.ReasoningEffort
	if reasoningEffort == "" {
		reasoningEffort = "xhigh"
	}

	timeoutMs := e.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 3600000
	}

	sandbox := e.Sandbox
	if sandbox == "" {
		sandbox = "read-only"
	}

	args := []string{
		"exec",
		"--sandbox", sandbox,
		"-c", fmt.Sprintf("model=%q", model),
		"-c", "model_reasoning_effort=" + reasoningEffort,
		"-c", fmt.Sprintf("stream_idle_timeout_ms=%d", timeoutMs),
	}

	if e.ProjectDoc != "" {
		args = append(args, "-c", fmt.Sprintf("project_doc=%q", e.ProjectDoc))
	}

	args = append(args, prompt)

	runner := e.runner
	if runner == nil {
		runner = &execRunner{dir: e.WorkingDir}
	}

	streams, wait, err := runner.Run(ctx, cmd, args...)
	if err != nil {
		return Result{Error: fmt.Errorf("start codex: %w", err)}
	}

	// process stderr for progress display (header block + bold summaries)
	stderrDone := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stderrDone <- fmt.Errorf("panic in processStderr: %v", r)
			}
		}()
		stderrDone <- e.processStderr(ctx, streams.Stderr)
	}()

	// read stdout entirely as final response
	stdoutContent, stdoutErr := e.readStdout(streams.Stdout)

	// wait for stderr processing to complete
	stderrErr := <-stderrDone

	// wait for command completion
	waitErr := wait()

	// determine final error (prefer stderr/stdout errors over wait error)
	var finalErr error
	switch {
	case stderrErr != nil && !errors.Is(stderrErr, context.Canceled):
		finalErr = stderrErr
	case stdoutErr != nil:
		finalErr = stdoutErr
	case waitErr != nil:
		if ctx.Err() != nil {
			finalErr = fmt.Errorf("context error: %w", ctx.Err())
		} else {
			finalErr = fmt.Errorf("codex exited with error: %w", waitErr)
		}
	}

	// detect signal in stdout (the actual response)
	signal := detectSignal(stdoutContent)

	// check for error patterns in output
	if pattern := checkErrorPatterns(stdoutContent, e.ErrorPatterns); pattern != "" {
		return Result{
			Output: stdoutContent,
			Signal: signal,
			Error:  &PatternMatchError{Pattern: pattern, HelpCmd: "codex /status"},
		}
	}

	return Result{Output: stdoutContent, Signal: signal, Error: finalErr}
}

// processStderr reads stderr line-by-line, filters for progress display.
// Shows header block (between first two "--------" separators) and bold summaries.
func (e *Executor) processStderr(ctx context.Context, r io.Reader) error {
	state := &filterState{}
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxScannerBuffer)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		default:
		}

		line := scanner.Text()
		if show, filtered := e.shouldDisplay(line, state); show {
			if e.OutputHandler != nil {
				e.OutputHandler(filtered + "\n")
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stderr: %w", err)
	}
	return nil
}

// maxStdoutSize is the maximum size of stdout content to read (64MB, matching stderr buffer).
const maxStdoutSize = 64 * 1024 * 1024

// readStdout reads the entire stdout content as the final response.
func (e *Executor) readStdout(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxStdoutSize+1))
	if err != nil {
		return "", fmt.Errorf("read stdout: %w", err)
	}
	if len(data) > maxStdoutSize {
		return "", fmt.Errorf("read stdout: exceeded maximum size of %d bytes", maxStdoutSize)
	}
	return string(data), nil
}

// shouldDisplay implements a simple filter for codex stderr output.
// Shows: header block (between first two "--------" separators) and bold summaries.
// Also deduplicates lines to avoid non-consecutive repeats.
func (e *Executor) shouldDisplay(line string, state *filterState) (bool, string) {
	s := strings.TrimSpace(line)
	if s == "" {
		return false, ""
	}

	var show bool
	var filtered string
	var skipDedup bool // separators are not deduplicated

	switch {
	case strings.HasPrefix(s, "--------"):
		state.headerCount++
		show = state.headerCount <= 2
		filtered = line
		skipDedup = true
	case state.headerCount == 1:
		// show everything between first two separators (header block)
		show = true
		filtered = line
	case strings.HasPrefix(s, "**"):
		// show bold summaries after header (progress indication)
		show = true
		filtered = stripBold(s)
	}

	// check for duplicates before returning (except separators)
	if show && !skipDedup {
		if state.seen == nil {
			state.seen = make(map[string]bool)
		}
		if state.seen[filtered] {
			return false, ""
		}
		state.seen[filtered] = true
	}

	return show, filtered
}

// stripBold removes markdown bold markers (**text**) from text.
func stripBold(s string) string {
	result := s
	for {
		start := strings.Index(result, "**")
		if start == -1 {
			break
		}
		end := strings.Index(result[start+2:], "**")
		if end == -1 {
			break
		}
		result = result[:start] + result[start+2:start+2+end] + result[start+2+end+2:]
	}
	return result
}

// detectSignal checks text for completion signals.
func detectSignal(text string) string {
	signals := []string{
		"<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>",
	}
	for _, sig := range signals {
		if strings.Contains(text, sig) {
			return sig
		}
	}
	return ""
}

// checkErrorPatterns checks output for configured error patterns.
// Returns the first matching pattern or empty string if none match.
// Matching is case-insensitive substring search.
func checkErrorPatterns(output string, patterns []string) string {
	if len(patterns) == 0 {
		return ""
	}
	outputLower := strings.ToLower(output)
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		if strings.Contains(outputLower, strings.ToLower(trimmed)) {
			return trimmed
		}
	}
	return ""
}
