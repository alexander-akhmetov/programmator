package codex

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/llm"
)

// Config holds environment configuration for codex subprocesses.
type Config struct {
	Model  string // -m value (e.g. "o3", "gpt-5-codex")
	APIKey string // OPENAI_API_KEY
}

// Invoker invokes the OpenAI Codex CLI binary.
type Invoker struct {
	Env Config
}

// New returns an Invoker that shells out to the "codex" binary.
func New(env Config) *Invoker {
	return &Invoker{Env: env}
}

// BuildEnv constructs the environment variable slice for a codex subprocess.
// It filters OPENAI_API_KEY from the inherited environment, then sets it
// from config if provided.
func BuildEnv(cfg Config) []string {
	env := llm.FilterEnv(os.Environ(), "OPENAI_API_KEY=")
	if cfg.APIKey != "" {
		env = append(env, "OPENAI_API_KEY="+cfg.APIKey)
	}
	return env
}

// Invoke runs codex with the given prompt and options.
func (c *Invoker) Invoke(ctx context.Context, prompt string, opts llm.InvokeOptions) (*llm.InvokeResult, error) {
	args := []string{"exec"}

	if c.Env.Model != "" {
		args = append(args, "-m", c.Env.Model)
	}

	if len(opts.ExtraFlags) > 0 {
		args = append(args, opts.ExtraFlags...)
	}

	if opts.Streaming {
		args = append(args, "--json")
	}

	if opts.WorkingDir != "" {
		args = append(args, "--cd", opts.WorkingDir)
	}

	// Prompt is the final positional argument.
	args = append(args, prompt)

	invokeCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		invokeCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(invokeCtx, "codex", args...)
	cmd.Env = BuildEnv(c.Env)

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

	if opts.OnSystemInit != nil && c.Env.Model != "" {
		opts.OnSystemInit(c.Env.Model)
	}

	var output string
	if opts.Streaming {
		output = processCodexStreamingOutput(stdout, c.Env.Model, opts)
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
			return nil, fmt.Errorf("codex exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("codex exited: %w", err)
	}

	return &llm.InvokeResult{Text: output}, nil
}
