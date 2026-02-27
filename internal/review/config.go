// Package review implements a single-loop code review pipeline.
package review

import (
	"github.com/alexander-akhmetov/programmator/internal/llm"
)

const (
	DefaultMaxIterations = 3
)

// Config holds the review configuration.
type Config struct {
	MaxIterations int           `yaml:"max_iterations"`
	Parallel      bool          `yaml:"parallel"`
	Timeout       int           `yaml:"-"` // seconds per agent invocation, inherited from main config
	Agents        []AgentConfig `yaml:"agents,omitempty"`
	ClaudeFlags   string        `yaml:"-"` // inherited from main config, not user-configured
	EnvConfig     llm.EnvConfig `yaml:"-"` // Claude subprocess environment (config dir, API key)
	TicketContext string        `yaml:"-"` // full ticket/plan content for reviewer context
}

// AgentConfig defines a single review agent configuration.
type AgentConfig struct {
	Name     string   `yaml:"name"`
	Executor string   `yaml:"executor,omitempty"` // executor type override (empty = use default)
	Focus    []string `yaml:"focus"`
	Prompt   string   `yaml:"prompt,omitempty"` // custom prompt path or empty for default
}

// DefaultConfig returns the default review configuration.
func DefaultConfig() Config {
	return Config{
		MaxIterations: DefaultMaxIterations,
		Parallel:      true,
		Agents:        DefaultAgents(),
	}
}

// DefaultAgents returns the default agent list for reviews.
func DefaultAgents() []AgentConfig {
	return []AgentConfig{
		{Name: "error-handling", Focus: []string{"error handling", "resource management", "concurrency", "race conditions"}},
		{Name: "logic", Focus: []string{"logic errors", "edge cases", "off-by-one", "incorrect conditionals", "nil handling"}},
		{Name: "security", Focus: []string{"input validation", "secrets", "injection"}},
		{Name: "implementation", Focus: []string{"requirement coverage", "wiring", "completeness"}},
		{Name: "testing", Focus: []string{"test coverage", "fake tests", "edge cases"}},
		{Name: "simplification", Focus: []string{"over-engineering", "unnecessary abstractions"}},
		{Name: "linter", Focus: []string{"lint errors", "formatting", "static analysis"}},
		{Name: "claudemd", Focus: []string{"CLAUDE.md compliance", "project conventions"}},
	}
}
