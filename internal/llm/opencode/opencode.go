package opencode

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

// Config holds environment configuration for opencode subprocesses.
type Config struct {
	Model     string // --model value ("provider/model" format, e.g. "anthropic/claude-sonnet-4-5")
	APIKey    string // provider API key, set based on model prefix
	ConfigDir string // OPENCODE_CONFIG_DIR
}

// Invoker invokes the opencode AI coding agent CLI binary.
type Invoker struct {
	Env Config
}

// New returns an Invoker that shells out to the "opencode" binary.
func New(env Config) *Invoker {
	return &Invoker{Env: env}
}

// ProviderFromModel extracts the provider prefix from a "provider/model" string.
// Returns empty string if no slash is found or model is empty.
func ProviderFromModel(model string) string {
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i]
	}
	return ""
}

// BuildEnv constructs the environment variable slice for an opencode subprocess.
// It filters OPENCODE_CONFIG_DIR and all known provider API keys from the inherited
// environment, then sets them based on the config.
func BuildEnv(cfg Config) []string {
	excludes := append(llm.AllProviderAPIKeyPrefixes(), "OPENCODE_CONFIG_DIR=")
	env := llm.FilterEnv(os.Environ(), excludes...)
	if cfg.ConfigDir != "" {
		env = append(env, "OPENCODE_CONFIG_DIR="+cfg.ConfigDir)
	}
	if cfg.APIKey != "" {
		provider := ProviderFromModel(cfg.Model)
		envVar := llm.ProviderAPIKeyEnvVars[provider]
		if envVar == "" {
			envVar = "ANTHROPIC_API_KEY" // default fallback
		}
		env = append(env, envVar+"="+cfg.APIKey)
	}
	return env
}

// Invoke runs opencode with the given prompt and options.
func (o *Invoker) Invoke(ctx context.Context, prompt string, opts llm.InvokeOptions) (*llm.InvokeResult, error) {
	args := []string{"run"}

	if o.Env.Model != "" {
		args = append(args, "--model", o.Env.Model)
	}

	if len(opts.ExtraFlags) > 0 {
		args = append(args, opts.ExtraFlags...)
	}

	if opts.Streaming {
		args = append(args, "--format", "json")
	}

	args = append(args, "-q")

	if opts.WorkingDir != "" {
		args = append(args, "--dir", opts.WorkingDir)
	}

	// Prompt is a positional argument, not stdin.
	args = append(args, prompt)

	invokeCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		invokeCtx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Second)
		defer cancel()
	}

	cmd := exec.CommandContext(invokeCtx, "opencode", args...)
	cmd.Env = BuildEnv(o.Env)

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

	// Fire OnSystemInit before processing output since opencode events don't contain model info.
	if opts.OnSystemInit != nil && o.Env.Model != "" {
		opts.OnSystemInit(o.Env.Model)
	}

	var output string
	if opts.Streaming {
		output = processOpenCodeStreamingOutput(stdout, o.Env.Model, opts)
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
			return nil, fmt.Errorf("opencode exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("opencode exited: %w", err)
	}

	return &llm.InvokeResult{Text: output}, nil
}
