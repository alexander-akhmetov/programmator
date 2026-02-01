// Package review implements a single-loop code review pipeline.
package review

import (
	"os"
	"strconv"
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
	SettingsJSON  string        `yaml:"-"` // pre-computed --settings JSON for guard mode
	TicketContext string        `yaml:"-"` // full ticket/plan content for reviewer context
	Codex         CodexSettings `yaml:"-"` // codex agent settings, injected from main config
}

// CodexSettings holds codex-specific settings passed to the CodexAgent.
type CodexSettings struct {
	Command         string
	Model           string
	ReasoningEffort string
	TimeoutMs       int
	Sandbox         string
	ProjectDoc      string
	ErrorPatterns   []string
}

// AgentConfig defines a single review agent configuration.
type AgentConfig struct {
	Name   string   `yaml:"name"`
	Focus  []string `yaml:"focus"`
	Prompt string   `yaml:"prompt,omitempty"` // custom prompt path or empty for default
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
		{Name: "codex", Focus: []string{"bugs", "security", "race conditions", "missing error handling", "resource leaks"}},
	}
}

// ConfigFromEnv loads config from environment or uses defaults.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxIterations = n
		}
	}

	return cfg
}
