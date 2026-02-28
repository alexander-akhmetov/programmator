package llm

import "fmt"

// ExecutorConfig selects and configures the LLM executor implementation.
type ExecutorConfig struct {
	Name       string            // "claude", "pi", "opencode", or "" (defaults to "claude")
	Claude     EnvConfig         // passed to ClaudeInvoker when Name is "claude"
	Pi         PiEnvConfig       // passed to PiInvoker when Name is "pi"
	OpenCode   OpenCodeEnvConfig // passed to OpenCodeInvoker when Name is "opencode"
	ExtraFlags []string          // additional CLI flags for the executor
}

// NewInvoker creates an Invoker based on the executor name in cfg.
// An empty Name defaults to "claude". Unknown names return an error.
func NewInvoker(cfg ExecutorConfig) (Invoker, error) {
	switch cfg.Name {
	case "claude", "":
		return NewClaudeInvoker(cfg.Claude), nil
	case "pi":
		return NewPiInvoker(cfg.Pi), nil
	case "opencode":
		return NewOpenCodeInvoker(cfg.OpenCode), nil
	default:
		return nil, fmt.Errorf("unknown executor: %q (supported: claude, pi, opencode)", cfg.Name)
	}
}
