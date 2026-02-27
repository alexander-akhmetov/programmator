package config

import (
	"log"
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
	agents := c.resolveReviewAgents()
	reviewFlags := ensureFlag(strings.Fields(c.Claude.Flags), "--dangerously-skip-permissions")
	return review.Config{
		MaxIterations: c.Review.MaxIterations,
		Parallel:      c.Review.Parallel,
		Timeout:       c.Timeout,
		ClaudeFlags:   strings.Join(reviewFlags, " "),
		Agents:        agents,
		EnvConfig: llm.EnvConfig{
			ClaudeConfigDir: c.Claude.ConfigDir,
			AnthropicAPIKey: c.Claude.AnthropicAPIKey,
		},
	}
}

// resolveReviewAgents returns the agent list, migrating from phases if needed.
func (c *Config) resolveReviewAgents() []review.AgentConfig {
	if len(c.Review.Agents) > 0 {
		if len(c.Review.Phases) > 0 {
			log.Printf("warning: review.phases is deprecated and ignored when review.agents is set; remove review.phases from config")
		}
		return c.Review.Agents
	}

	// Migrate from phases: flatten all phase agents into a single list
	if len(c.Review.Phases) > 0 {
		log.Printf("warning: review.phases is deprecated; migrate to review.agents (flat agent list)")
		return flattenPhaseAgents(c.Review.Phases)
	}

	return nil
}

// flattenPhaseAgents extracts unique agents from all phases (dedup by name, first wins).
func flattenPhaseAgents(phases []ReviewPhase) []review.AgentConfig {
	seen := make(map[string]struct{})
	var result []review.AgentConfig
	for _, p := range phases {
		for _, a := range p.Agents {
			if _, ok := seen[a.Name]; ok {
				continue
			}
			seen[a.Name] = struct{}{}
			result = append(result, review.AgentConfig{
				Name:     a.Name,
				Executor: a.Executor,
				Focus:    a.Focus,
				Prompt:   a.Prompt,
			})
		}
	}
	return result
}
