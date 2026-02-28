// Package executor provides a factory for constructing LLM invokers by name.
// It imports the concrete executor subpackages (claude, pi, opencode) and
// selects the appropriate one based on Config.Name.
package executor

import (
	"fmt"

	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/llm/claude"
	"github.com/alexander-akhmetov/programmator/internal/llm/codex"
	"github.com/alexander-akhmetov/programmator/internal/llm/opencode"
	"github.com/alexander-akhmetov/programmator/internal/llm/pi"
)

// Config selects and configures the LLM executor implementation.
type Config struct {
	Name       string          // "claude", "pi", "opencode", "codex", or "" (defaults to "claude")
	Claude     claude.Config   // passed to claude.New when Name is "claude"
	Pi         pi.Config       // passed to pi.New when Name is "pi"
	OpenCode   opencode.Config // passed to opencode.New when Name is "opencode"
	Codex      codex.Config    // passed to codex.New when Name is "codex"
	ExtraFlags []string        // additional CLI flags for the executor
}

// New creates an Invoker based on the executor name in cfg.
// An empty Name defaults to "claude". Unknown names return an error.
func New(cfg Config) (llm.Invoker, error) {
	switch cfg.Name {
	case "claude", "":
		return claude.New(cfg.Claude), nil
	case "pi":
		return pi.New(cfg.Pi), nil
	case "opencode":
		return opencode.New(cfg.OpenCode), nil
	case "codex":
		return codex.New(cfg.Codex), nil
	default:
		return nil, fmt.Errorf("unknown executor: %q (supported: claude, pi, opencode, codex)", cfg.Name)
	}
}
