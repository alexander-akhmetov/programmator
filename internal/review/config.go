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
	MaxIterations  int                `yaml:"max_iterations"`
	Parallel       bool               `yaml:"parallel"`
	Timeout        int                `yaml:"-"` // seconds per agent invocation, inherited from main config
	Agents         []AgentConfig      `yaml:"agents,omitempty"`
	ExecutorConfig llm.ExecutorConfig `yaml:"-"` // executor configuration, inherited from main config
	TicketContext  string             `yaml:"-"` // full ticket/plan content for reviewer context
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
		{Name: "bug-shallow", Focus: []string{"obvious bugs visible in diff only"}},
		{Name: "bug-deep", Focus: []string{"bugs in introduced code requiring context", "security", "resource leaks", "concurrency"}},
		{Name: "architect", Focus: []string{"architectural fit", "better approaches", "coupling"}},
		{Name: "simplification", Focus: []string{"over-engineering", "unnecessary complexity"}},
		{Name: "silent-failures", Focus: []string{"silent failures", "swallowed errors", "inadequate logging"}},
		{Name: "claudemd", Focus: []string{"CLAUDE.md compliance"}},
		{Name: "type-design", Focus: []string{"type/interface design quality"}},
		{Name: "comments", Focus: []string{"comment accuracy and value"}},
		{Name: "tests-and-linters", Focus: []string{"test failures", "lint errors", "formatting"}},
	}
}
