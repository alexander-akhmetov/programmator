package config

import (
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// ToSafetyConfig converts the unified Config to a safety.Config.
// This provides backwards compatibility during the migration.
func (c *Config) ToSafetyConfig() safety.Config {
	return safety.Config{
		MaxIterations:       c.MaxIterations,
		StagnationLimit:     c.StagnationLimit,
		Timeout:             c.Timeout,
		ClaudeFlags:         c.ClaudeFlags,
		ClaudeConfigDir:     c.ClaudeConfigDir,
		MaxReviewIterations: c.Review.MaxIterations,
	}
}

// ToReviewConfig converts the unified Config to a review.Config.
// This provides backwards compatibility during the migration.
func (c *Config) ToReviewConfig() review.Config {
	passes := make([]review.Pass, len(c.Review.Passes))
	for i, p := range c.Review.Passes {
		agents := make([]review.AgentConfig, len(p.Agents))
		for j, a := range p.Agents {
			agents[j] = review.AgentConfig{
				Name:   a.Name,
				Focus:  a.Focus,
				Prompt: a.Prompt,
			}
		}
		passes[i] = review.Pass{
			Name:     p.Name,
			Parallel: p.Parallel,
			Agents:   agents,
		}
	}

	return review.Config{
		Enabled:       c.Review.Enabled,
		MaxIterations: c.Review.MaxIterations,
		Passes:        passes,
	}
}
