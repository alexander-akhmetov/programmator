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
	DefaultEnabled       = false
)

// Config holds the review configuration.
type Config struct {
	Enabled       bool   `yaml:"enabled"`
	MaxIterations int    `yaml:"max_iterations"`
	Passes        []Pass `yaml:"passes"`
}

// Pass defines a review pass with multiple agents.
type Pass struct {
	Name     string  `yaml:"name"`
	Parallel bool    `yaml:"parallel"`
	Agents   []Agent `yaml:"agents"`
}

// Agent defines a single review agent.
type Agent struct {
	Name   string   `yaml:"name"`
	Focus  []string `yaml:"focus"`
	Prompt string   `yaml:"prompt,omitempty"` // custom prompt path or empty for default
}

// DefaultConfig returns the default review configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:       DefaultEnabled,
		MaxIterations: DefaultMaxIterations,
		Passes: []Pass{
			{
				Name:     "code_review",
				Parallel: true,
				Agents: []Agent{
					{
						Name:  "quality",
						Focus: []string{"error handling", "code clarity", "test coverage"},
					},
					{
						Name:  "security",
						Focus: []string{"input validation", "secrets", "injection"},
					},
				},
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

	// Check for review enabled via env
	if v := os.Getenv("PROGRAMMATOR_REVIEW_ENABLED"); v != "" {
		cfg.Enabled = v == "true" || v == "1"
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

	// If file has no passes defined, use default
	if len(result.Passes) == 0 {
		result.Passes = env.Passes
	}

	return result
}
