package pi

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

// Config holds environment configuration for pi subprocesses.
type Config struct {
	ConfigDir string // PI_CODING_AGENT_DIR
	Provider  string // --provider value (e.g. "anthropic", "openai")
	Model     string // --model value (e.g. "sonnet", "gpt-4o")
	APIKey    string // API key for the configured provider
}

// Invoker invokes the pi coding agent CLI binary.
type Invoker struct {
	Env Config
}

// New returns an Invoker that shells out to the "pi" binary.
func New(env Config) *Invoker {
	return &Invoker{Env: env}
}

// BuildEnv constructs the environment variable slice for a pi subprocess.
// It filters all known provider API keys from the inherited environment to
// prevent key leakage, then sets PI_CODING_AGENT_DIR and the provider-specific
// API key if configured.
func BuildEnv(cfg Config) []string {
	excludes := append(llm.AllProviderAPIKeyPrefixes(), "PI_CODING_AGENT_DIR=")
	env := llm.FilterEnv(os.Environ(), excludes...)
	if cfg.ConfigDir != "" {
		env = append(env, "PI_CODING_AGENT_DIR="+cfg.ConfigDir)
	}
	if cfg.APIKey != "" {
		envVar := llm.ProviderAPIKeyEnvVars[cfg.Provider]
		if envVar == "" {
			envVar = "ANTHROPIC_API_KEY" // default to anthropic
		}
		env = append(env, envVar+"="+cfg.APIKey)
	}
	return env
}

// Invoke runs pi with the given prompt and options.
func (p *Invoker) Invoke(ctx context.Context, prompt string, opts llm.InvokeOptions) (*llm.InvokeResult, error) {
	var args []string

	if p.Env.Provider != "" {
		args = append(args, "--provider", p.Env.Provider)
	}
	if p.Env.Model != "" {
		args = append(args, "--model", p.Env.Model)
	}

	if len(opts.ExtraFlags) > 0 {
		args = append(args, opts.ExtraFlags...)
	}

	if opts.Streaming {
		args = append(args, "--mode", "json")
	} else {
		args = append(args, "--print")
	}

	invokeCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		invokeCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(invokeCtx, "pi", args...)
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	cmd.Env = BuildEnv(p.Env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

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
		output = processPiStreamingOutput(stdout, opts)
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
			return nil, fmt.Errorf("pi exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("pi exited: %w", err)
	}

	return &llm.InvokeResult{Text: output}, nil
}
