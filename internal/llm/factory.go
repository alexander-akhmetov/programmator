package llm

import "fmt"

// ExecutorConfig selects and configures the LLM executor implementation.
type ExecutorConfig struct {
	Name   string    // "claude" (only supported value for now)
	Claude EnvConfig // passed to ClaudeInvoker when Name is "claude"
}

// NewInvoker creates an Invoker based on the executor name in cfg.
// An empty Name defaults to "claude". Unknown names return an error.
func NewInvoker(cfg ExecutorConfig) (Invoker, error) {
	switch cfg.Name {
	case "claude", "":
		return NewClaudeInvoker(cfg.Claude), nil
	default:
		return nil, fmt.Errorf("unknown executor: %q (supported: claude)", cfg.Name)
	}
}
