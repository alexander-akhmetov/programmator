// Package review implements a multi-phase code review pipeline.
package review

import (
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	DefaultMaxIterations = 3
)

// Config holds the review configuration.
type Config struct {
	MaxIterations int     `yaml:"max_iterations"`
	Timeout       int     `yaml:"-"` // seconds per agent invocation, inherited from main config
	Phases        []Phase `yaml:"phases,omitempty"`
	ClaudeFlags   string  `yaml:"-"` // inherited from main config, not user-configured
	SettingsJSON  string  `yaml:"-"` // pre-computed --settings JSON for guard mode
}

// AgentConfig defines a single review agent configuration.
type AgentConfig struct {
	Name   string   `yaml:"name"`
	Focus  []string `yaml:"focus"`
	Prompt string   `yaml:"prompt,omitempty"` // custom prompt path or empty for default
}

// Phase defines a review phase with iteration limits and severity filtering.
// Phases are the new multi-phase review system, replacing passes for more control.
type Phase struct {
	Name           string        `yaml:"name"`
	IterationLimit int           `yaml:"iteration_limit,omitempty"` // static limit (takes precedence)
	IterationPct   int           `yaml:"iteration_pct,omitempty"`   // % of max_iterations (min 3)
	SeverityFilter []Severity    `yaml:"severity_filter,omitempty"` // empty = all severities
	Agents         []AgentConfig `yaml:"agents"`
	Parallel       bool          `yaml:"parallel"`
}

// MaxIterations returns the maximum iterations allowed for this phase.
// If IterationLimit is set, it takes precedence.
// Otherwise, IterationPct is used as a percentage of globalMax (minimum 3).
// If neither is set, returns 3 as default.
func (p *Phase) MaxIterations(globalMax int) int {
	if p.IterationLimit > 0 {
		return p.IterationLimit
	}
	if p.IterationPct > 0 {
		calculated := globalMax * p.IterationPct / 100
		if calculated < 3 {
			return 3
		}
		return calculated
	}
	return 3 // default
}

// DefaultConfig returns the default review configuration.
func DefaultConfig() Config {
	return Config{
		MaxIterations: DefaultMaxIterations,
		Phases:        DefaultPhases(),
	}
}

// DefaultPhases returns the default multi-phase review configuration.
func DefaultPhases() []Phase {
	return []Phase{
		{
			Name:           "comprehensive",
			IterationLimit: 1,
			Parallel:       true,
			Agents: []AgentConfig{
				{Name: "quality", Focus: []string{"error handling", "code clarity", "resource management", "concurrency"}},
				{Name: "security", Focus: []string{"input validation", "secrets", "injection"}},
				{Name: "implementation", Focus: []string{"requirement coverage", "wiring", "completeness"}},
				{Name: "testing", Focus: []string{"test coverage", "fake tests", "edge cases"}},
				{Name: "simplification", Focus: []string{"over-engineering", "unnecessary abstractions"}},
				{Name: "linter", Focus: []string{"lint errors", "formatting", "static analysis"}},
			},
		},
		{
			Name:           "critical_loop",
			IterationPct:   10, // max(3, maxIter*10/100)
			SeverityFilter: []Severity{SeverityCritical, SeverityHigh},
			Parallel:       true,
			Agents: []AgentConfig{
				{Name: "quality", Focus: []string{"bugs", "security", "race conditions", "error handling"}},
				{Name: "implementation", Focus: []string{"correctness", "completeness"}},
			},
		},
		{
			Name:           "final_check",
			IterationPct:   10,
			SeverityFilter: []Severity{SeverityCritical, SeverityHigh},
			Parallel:       true,
			Agents: []AgentConfig{
				{Name: "quality", Focus: []string{"bugs", "security", "race conditions", "error handling"}},
				{Name: "implementation", Focus: []string{"correctness", "completeness"}},
			},
		},
	}
}

// ConfigFromEnv loads config from file or uses defaults.
// Checks for PROGRAMMATOR_REVIEW_CONFIG env var or ~/.programmator/review.yaml.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	// Check for custom max iterations via env
	if v := os.Getenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxIterations = n
		}
	}

	// Try to load config file
	configPath := os.Getenv("PROGRAMMATOR_REVIEW_CONFIG")
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			configPath = filepath.Join(home, ".programmator", "review.yaml")
		}
	}

	if configPath != "" {
		if fileConfig, err := LoadConfigFile(configPath); err == nil {
			cfg = mergeConfigs(cfg, fileConfig)
		}
	}

	return cfg
}

// LoadConfigFile loads a review configuration from a YAML file.
func LoadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// mergeConfigs merges file config over env defaults.
// File values take precedence when set.
func mergeConfigs(env, file Config) Config {
	result := file

	// Keep env overrides if file doesn't specify
	if result.MaxIterations == 0 {
		result.MaxIterations = env.MaxIterations
	}

	// If file has no phases defined, use default
	if len(result.Phases) == 0 {
		result.Phases = env.Phases
	}

	return result
}
