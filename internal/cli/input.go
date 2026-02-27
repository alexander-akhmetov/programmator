package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Collector provides interactive input collection for plan creation.
type Collector interface {
	// AskQuestion presents a question with options and returns the selected answer.
	AskQuestion(ctx context.Context, question string, options []string) (string, error)
}

// TerminalCollector implements Collector using fzf (if available) or numbered selection fallback.
type TerminalCollector struct {
	stdin  io.Reader // for testing, nil uses os.Stdin
	stdout io.Writer // for testing, nil uses os.Stdout
}

// NewTerminalCollector creates a new TerminalCollector with default stdin/stdout.
func NewTerminalCollector() *TerminalCollector {
	return &TerminalCollector{}
}

// NewTerminalCollectorWithIO creates a TerminalCollector with custom I/O (for testing).
func NewTerminalCollectorWithIO(stdin io.Reader, stdout io.Writer) *TerminalCollector {
	return &TerminalCollector{
		stdin:  stdin,
		stdout: stdout,
	}
}

// AskQuestion presents options using fzf if available, otherwise falls back to numbered selection.
func (c *TerminalCollector) AskQuestion(ctx context.Context, question string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options provided")
	}

	if hasFzf() {
		return c.selectWithFzf(ctx, question, options)
	}

	return c.selectWithNumbers(question, options)
}

// hasFzf checks if fzf is available in PATH.
func hasFzf() bool {
	_, err := exec.LookPath("fzf")
	return err == nil
}

// selectWithFzf uses fzf for interactive selection.
func (c *TerminalCollector) selectWithFzf(ctx context.Context, question string, options []string) (string, error) {
	input := strings.Join(options, "\n")

	cmd := exec.CommandContext(ctx, "fzf", "--prompt", question+": ", "--height", "10", "--layout=reverse") //nolint:gosec // fzf is a trusted external tool
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 130 {
			return "", errors.New("selection canceled")
		}
		return "", fmt.Errorf("fzf selection failed: %w", err)
	}

	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return "", errors.New("no selection made")
	}

	return selected, nil
}

// selectWithNumbers presents numbered options for selection via stdin.
func (c *TerminalCollector) selectWithNumbers(question string, options []string) (string, error) {
	stdout := c.stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stdin := c.stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, question)
	for i, opt := range options {
		_, _ = fmt.Fprintf(stdout, "  %d) %s\n", i+1, opt)
	}
	_, _ = fmt.Fprintf(stdout, "Enter number (1-%d): ", len(options))

	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", errors.New("input stream closed")
		}
		return "", fmt.Errorf("read input: %w", err)
	}

	line = strings.TrimSpace(line)
	num, err := strconv.Atoi(line)
	if err != nil {
		return "", fmt.Errorf("invalid number: %s", line)
	}

	if num < 1 || num > len(options) {
		return "", fmt.Errorf("selection out of range: %d (must be 1-%d)", num, len(options))
	}

	return options[num-1], nil
}
