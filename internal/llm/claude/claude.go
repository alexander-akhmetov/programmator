package claude

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/alexander-akhmetov/programmator/internal/llm"
)

// Config holds environment configuration for Claude subprocesses.
type Config struct {
	ClaudeConfigDir string
	AnthropicAPIKey string
}

// Invoker invokes the Claude CLI binary.
type Invoker struct {
	Env Config
}

// New returns an Invoker that shells out to the "claude" binary.
func New(env Config) *Invoker {
	return &Invoker{Env: env}
}

// BuildEnv constructs the environment variable slice for a Claude subprocess.
// It filters ANTHROPIC_API_KEY and CLAUDE_CONFIG_DIR from the inherited
// environment and only sets them if explicitly configured via the Config.
func BuildEnv(cfg Config) []string {
	env := llm.FilterEnv(os.Environ(), "ANTHROPIC_API_KEY=", "CLAUDE_CONFIG_DIR=")
	if cfg.ClaudeConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+cfg.ClaudeConfigDir)
	}
	if cfg.AnthropicAPIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+cfg.AnthropicAPIKey)
	}
	return env
}

// Invoke runs claude --print with the given prompt and options.
func (c *Invoker) Invoke(ctx context.Context, prompt string, opts llm.InvokeOptions) (*llm.InvokeResult, error) {
	args := []string{"--print"}

	if len(opts.ExtraFlags) > 0 {
		args = append(args, opts.ExtraFlags...)
	}

	if opts.Streaming {
		args = append(args, "--output-format", "stream-json", "--verbose")
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
		output = llm.ProcessTextOutput(stdout, opts)
	}

	err = cmd.Wait()
	if opts.OnProcessEnd != nil {
		opts.OnProcessEnd()
	}
	if err != nil {
		if invokeCtx.Err() == context.DeadlineExceeded {
			return &llm.InvokeResult{Text: llm.TimeoutBlockedStatus()}, nil
		}
		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			return nil, fmt.Errorf("claude exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("claude exited: %w", err)
	}

	return &llm.InvokeResult{Text: output}, nil
}
