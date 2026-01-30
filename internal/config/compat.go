package config

import (
	"github.com/worksonmyai/programmator/internal/review"
	"github.com/worksonmyai/programmator/internal/safety"
)

// ToSafetyConfig converts the unified Config to a safety.Config.
func (c *Config) ToSafetyConfig() safety.Config {
	return safety.Config{
		MaxIterations:       c.MaxIterations,
		StagnationLimit:     c.StagnationLimit,
		Timeout:             c.Timeout,
		ClaudeFlags:         c.ClaudeFlags,
		ClaudeConfigDir:     c.ClaudeConfigDir,
		AnthropicAPIKey:     c.AnthropicAPIKey,
		MaxReviewIterations: c.Review.MaxIterations,
	}
}

// ToReviewConfig converts the unified Config to a review.Config.
func (c *Config) ToReviewConfig() review.Config {
	phases := make([]review.Phase, len(c.Review.Phases))
	for i, p := range c.Review.Phases {
		agents := make([]review.AgentConfig, len(p.Agents))
		for j, a := range p.Agents {
			agents[j] = review.AgentConfig{
				Name:   a.Name,
				Focus:  a.Focus,
				Prompt: a.Prompt,
			}
		}

		severities := make([]review.Severity, len(p.SeverityFilter))
		for j, s := range p.SeverityFilter {
			severities[j] = review.Severity(s)
		}

		phases[i] = review.Phase{
			Name:           p.Name,
			IterationLimit: p.IterationLimit,
			IterationPct:   p.IterationPct,
			SeverityFilter: severities,
			Agents:         agents,
			Parallel:       p.Parallel,
		}
	}

	return review.Config{
		MaxIterations: c.Review.MaxIterations,
		Phases:        phases,
	}
}
