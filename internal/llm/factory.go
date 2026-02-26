package llm

import "fmt"

// ExecutorConfig selects and configures the LLM executor implementation.
type ExecutorConfig struct {
	Name       string    // "claude" (only supported value for now)
	Claude     EnvConfig // passed to ClaudeInvoker when Name is "claude"
	ExtraFlags []string  // additional CLI flags for the executor

	// Hook settings — set dynamically at runtime (e.g. after TUI permission
	// server starts). ClaudeInvoker auto-builds --settings from these when
	// InvokeOptions.SettingsJSON is empty.
	PermissionSocketPath string
	GuardMode            bool
}

// NewInvoker creates an Invoker based on the executor name in cfg.
// An empty Name defaults to "claude". Unknown names return an error.
func NewInvoker(cfg ExecutorConfig) (Invoker, error) {
	switch cfg.Name {
	case "claude", "":
		return NewClaudeInvoker(cfg.Claude, cfg.PermissionSocketPath, cfg.GuardMode), nil
	default:
		return nil, fmt.Errorf("unknown executor: %q (supported: claude)", cfg.Name)
	}
}
