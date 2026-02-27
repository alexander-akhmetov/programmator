package config

import (
	"slices"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// ToExecutorConfig converts the unified Config to an llm.ExecutorConfig.
// Always injects --dangerously-skip-permissions because the permission system
// has been removed; dcg is the sole safety layer.
func (c *Config) ToExecutorConfig() llm.ExecutorConfig {
	flags := strings.Fields(c.Claude.Flags)
	flags = ensureFlag(flags, "--dangerously-skip-permissions")
	return llm.ExecutorConfig{
		Name: c.Executor,
		Claude: llm.EnvConfig{
			ClaudeConfigDir: c.Claude.ConfigDir,
			AnthropicAPIKey: c.Claude.AnthropicAPIKey,
		},
		ExtraFlags: flags,
	}
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
	reviewFlags := ensureFlag(strings.Fields(c.Claude.Flags), "--dangerously-skip-permissions")
	return review.Config{
		MaxIterations: c.Review.MaxIterations,
		Parallel:      c.Review.Parallel,
		Timeout:       c.Timeout,
		ClaudeFlags:   strings.Join(reviewFlags, " "),
		Agents:        c.Review.Agents,
		EnvConfig: llm.EnvConfig{
			ClaudeConfigDir: c.Claude.ConfigDir,
			AnthropicAPIKey: c.Claude.AnthropicAPIKey,
		},
	}
}
