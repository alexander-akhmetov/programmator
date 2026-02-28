package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// OpenCodeInvoker invokes the opencode AI coding agent CLI binary.
type OpenCodeInvoker struct {
	Env OpenCodeEnvConfig
}

// OpenCodeEnvConfig holds environment configuration for opencode subprocesses.
type OpenCodeEnvConfig struct {
	Model     string // --model value ("provider/model" format, e.g. "anthropic/claude-sonnet-4-5")
	APIKey    string // provider API key, set based on model prefix
	ConfigDir string // OPENCODE_CONFIG_DIR
}

// NewOpenCodeInvoker returns an Invoker that shells out to the "opencode" binary.
func NewOpenCodeInvoker(env OpenCodeEnvConfig) *OpenCodeInvoker {
	return &OpenCodeInvoker{Env: env}
}

// Invoke runs opencode with the given prompt and options.
func (o *OpenCodeInvoker) Invoke(ctx context.Context, prompt string, opts InvokeOptions) (*InvokeResult, error) {
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
	cmd.Env = BuildOpenCodeEnv(o.Env)

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
		output = ProcessTextOutput(stdout, opts)
	}

	err = cmd.Wait()
	if opts.OnProcessEnd != nil {
		opts.OnProcessEnd()
	}
	if err != nil {
		if invokeCtx.Err() == context.DeadlineExceeded {
			return &InvokeResult{Text: TimeoutBlockedStatus()}, nil
		}
		if stderrStr := strings.TrimSpace(stderrBuf.String()); stderrStr != "" {
			return nil, fmt.Errorf("opencode exited: %w\nstderr: %s", err, stderrStr)
		}
		return nil, fmt.Errorf("opencode exited: %w", err)
	}

	return &InvokeResult{Text: output}, nil
}

// providerFromModel extracts the provider prefix from a "provider/model" string.
// Returns empty string if no slash is found or model is empty.
func providerFromModel(model string) string {
	if i := strings.Index(model, "/"); i > 0 {
		return model[:i]
	}
	return ""
}

// BuildOpenCodeEnv constructs the environment variable slice for an opencode subprocess.
// It filters OPENCODE_CONFIG_DIR and all known provider API keys from the inherited
// environment, then sets them based on the config.
func BuildOpenCodeEnv(cfg OpenCodeEnvConfig) []string {
	excludes := append(AllProviderAPIKeyPrefixes(), "OPENCODE_CONFIG_DIR=")
	env := FilterEnv(os.Environ(), excludes...)
	if cfg.ConfigDir != "" {
		env = append(env, "OPENCODE_CONFIG_DIR="+cfg.ConfigDir)
	}
	if cfg.APIKey != "" {
		provider := providerFromModel(cfg.Model)
		envVar := ProviderAPIKeyEnvVars[provider]
		if envVar == "" {
			envVar = "ANTHROPIC_API_KEY" // default fallback
		}
		env = append(env, envVar+"="+cfg.APIKey)
	}
	return env
}
