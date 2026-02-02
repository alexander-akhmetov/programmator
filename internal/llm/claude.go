package llm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

// ClaudeInvoker invokes the Claude CLI binary.
type ClaudeInvoker struct {
	Env EnvConfig
}

// NewClaudeInvoker returns an Invoker that shells out to the "claude" binary.
func NewClaudeInvoker(env EnvConfig) *ClaudeInvoker {
	return &ClaudeInvoker{Env: env}
}

// Invoke runs claude --print with the given prompt and options.
func (c *ClaudeInvoker) Invoke(ctx context.Context, prompt string, opts InvokeOptions) (*InvokeResult, error) {
	args := []string{"--print"}

	if opts.ExtraFlags != "" {
		args = append(args, strings.Fields(opts.ExtraFlags)...)
	}

	if opts.Streaming {
		args = append(args, "--output-format", "stream-json", "--verbose")
	}

	if opts.SettingsJSON != "" {
		args = append(args, "--settings", opts.SettingsJSON)
	}

	invokeCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		invokeCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(invokeCtx, "claude", args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	cmd.Env = BuildEnv(c.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	if opts.OnProcessStart != nil {
		opts.OnProcessStart(cmd.Process.Pid)
	}

	go func() {
		defer stdin.Close()
		if _, err := io.WriteString(stdin, prompt); err != nil {
			debug.Logf("failed to write prompt to stdin: %v", err)
		}
	}()

	var output string
	if opts.Streaming {
		output = processStreamingOutput(stdout, opts)
	} else {
		output = processTextOutput(stdout, opts)
	}

	err = cmd.Wait()
	if opts.OnProcessEnd != nil {
		opts.OnProcessEnd()
	}
	if err != nil {
		if invokeCtx.Err() == context.DeadlineExceeded {
			return &InvokeResult{Text: timeoutBlockedStatus()}, nil
		}
		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			return nil, fmt.Errorf("claude exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("claude exited: %w", err)
	}

	return &InvokeResult{Text: output}, nil
}

func timeoutBlockedStatus() string {
	return protocol.StatusBlockKey + `:
  phase_completed: ` + protocol.NullPhase + `
  status: ` + string(protocol.StatusBlocked) + `
  files_changed: []
  summary: "Timeout"
  error: "Claude invocation timed out"`
}
