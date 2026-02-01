package codex

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Streams holds both stderr and stdout from codex command.
type Streams struct {
	Stderr io.Reader
	Stdout io.Reader
}

// Runner abstracts command execution for codex.
// Returns both stderr (streaming progress) and stdout (final response).
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (streams Streams, wait func() error, err error)
}

// execRunner is the default command runner using os/exec for codex.
// Codex outputs streaming progress to stderr, final response to stdout.
type execRunner struct {
	dir string // working directory; empty uses process cwd
}

func (r *execRunner) Run(ctx context.Context, name string, args ...string) (Streams, func() error, error) {
	if err := ctx.Err(); err != nil {
		return Streams{}, nil, fmt.Errorf("context already canceled: %w", err)
	}

	// use exec.Command (not CommandContext) because we handle cancellation ourselves
	// to ensure the entire process group is killed, not just the direct child
	cmd := exec.Command(name, args...) //nolint:gosec // command and args are controlled by config
	cmd.Dir = r.dir

	setupProcessGroup(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Streams{}, nil, fmt.Errorf("stderr pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stderr.Close()
		return Streams{}, nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stderr.Close()
		stdout.Close()
		return Streams{}, nil, fmt.Errorf("start command: %w", err)
	}

	cleanup := newProcessGroupCleanup(cmd, ctx.Done())

	return Streams{Stderr: stderr, Stdout: stdout}, cleanup.Wait, nil
}
