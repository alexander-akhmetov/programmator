package config

import (
	"slices"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// ToExecutorConfig converts the unified Config to an llm.ExecutorConfig.
// For Claude, always injects --dangerously-skip-permissions because the
// permission system has been removed; dcg is the sole safety layer.
func (c *Config) ToExecutorConfig() llm.ExecutorConfig {
	cfg := llm.ExecutorConfig{Name: c.Executor}

	switch c.Executor {
	case "pi":
		cfg.Pi = llm.PiEnvConfig{
			ConfigDir: c.Pi.ConfigDir,
			Provider:  c.Pi.Provider,
			Model:     c.Pi.Model,
			APIKey:    c.Pi.APIKey,
		}
		cfg.ExtraFlags = strings.Fields(c.Pi.Flags)
	default: // "claude" or ""
		cfg.Claude = llm.EnvConfig{
			ClaudeConfigDir: c.Claude.ConfigDir,
			AnthropicAPIKey: c.Claude.AnthropicAPIKey,
		}
		flags := strings.Fields(c.Claude.Flags)
		cfg.ExtraFlags = ensureFlag(flags, "--dangerously-skip-permissions")
	}

	return cfg
}

func ensureFlag(flags []string, flag string) []string {
	if slices.Contains(flags, flag) {
		return flags
	}
	return append(flags, flag)
}

// ToSafetyConfig converts the unified Config to a safety.Config.
func (c *Config) ToSafetyConfig() safety.Config {
	return safety.Config{
		MaxIterations:       c.MaxIterations,
		StagnationLimit:     c.StagnationLimit,
		Timeout:             c.Timeout,
		MaxReviewIterations: c.Review.MaxIterations,
	}
}

// ToReviewConfig converts the unified Config to a review.Config.
func (c *Config) ToReviewConfig() review.Config {
	return review.Config{
		MaxIterations:  c.Review.MaxIterations,
		Parallel:       c.Review.Parallel,
		Timeout:        c.Timeout,
		Agents:         c.Review.Agents,
		ExecutorConfig: c.ToExecutorConfig(),
	}
}
