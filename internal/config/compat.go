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
	return review.NewConfigFrom(review.ConfigParams{
		MaxIterations: c.Review.MaxIterations,
		Timeout:       c.Timeout,
		ClaudeFlags:   c.ClaudeFlags,
		Phases:        convertReviewPhases(c.Review.Phases),
	})
}

// convertReviewPhases converts config review phases to review package phases.
func convertReviewPhases(phases []ReviewPhase) []review.Phase {
	result := make([]review.Phase, len(phases))
	for i, p := range phases {
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

		result[i] = review.Phase{
			Name:           p.Name,
			IterationLimit: p.IterationLimit,
			IterationPct:   p.IterationPct,
			SeverityFilter: severities,
			Agents:         agents,
			Parallel:       p.Parallel,
		}
	}
	return result
}
