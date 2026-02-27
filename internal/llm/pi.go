package llm

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
)

// PiInvoker invokes the pi coding agent CLI binary.
type PiInvoker struct {
	Env PiEnvConfig
}

// PiEnvConfig holds environment configuration for pi subprocesses.
type PiEnvConfig struct {
	ConfigDir string // PI_CODING_AGENT_DIR
	Provider  string // --provider value (e.g. "anthropic", "openai")
	Model     string // --model value (e.g. "sonnet", "gpt-4o")
	APIKey    string // API key for the configured provider
}

// NewPiInvoker returns an Invoker that shells out to the "pi" binary.
func NewPiInvoker(env PiEnvConfig) *PiInvoker {
	return &PiInvoker{Env: env}
}

// Invoke runs pi with the given prompt and options.
func (p *PiInvoker) Invoke(ctx context.Context, prompt string, opts InvokeOptions) (*InvokeResult, error) {
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

	cmd.Env = BuildPiEnv(p.Env)

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
			return nil, fmt.Errorf("pi exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("pi exited: %w", err)
	}

	return &InvokeResult{Text: output}, nil
}

// providerAPIKeyEnvVars maps pi provider names to their expected env var.
var providerAPIKeyEnvVars = map[string]string{
	"anthropic": "ANTHROPIC_API_KEY",
	"openai":    "OPENAI_API_KEY",
	"google":    "GEMINI_API_KEY",
	"groq":      "GROQ_API_KEY",
	"mistral":   "MISTRAL_API_KEY",
}

// allProviderAPIKeyPrefixes returns all known provider API key env var prefixes for filtering.
func allProviderAPIKeyPrefixes() []string {
	prefixes := make([]string, 0, len(providerAPIKeyEnvVars))
	for _, v := range providerAPIKeyEnvVars {
		prefixes = append(prefixes, v+"=")
	}
	return prefixes
}

// BuildPiEnv constructs the environment variable slice for a pi subprocess.
// It filters all known provider API keys from the inherited environment to
// prevent key leakage, then sets PI_CODING_AGENT_DIR and the provider-specific
// API key if configured.
func BuildPiEnv(cfg PiEnvConfig) []string {
	prefixes := allProviderAPIKeyPrefixes()
	environ := os.Environ()
	env := make([]string, 0, len(environ))
	for _, e := range environ {
		filtered := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(e, prefix) {
				filtered = true
				break
			}
		}
		if !filtered && !strings.HasPrefix(e, "PI_CODING_AGENT_DIR=") {
			env = append(env, e)
		}
	}
	if cfg.ConfigDir != "" {
		env = append(env, "PI_CODING_AGENT_DIR="+cfg.ConfigDir)
	}
	if cfg.APIKey != "" {
		envVar := providerAPIKeyEnvVars[cfg.Provider]
		if envVar == "" {
			envVar = "ANTHROPIC_API_KEY" // default to anthropic
		}
		env = append(env, envVar+"="+cfg.APIKey)
	}
	return env
}
