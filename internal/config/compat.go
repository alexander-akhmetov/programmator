package config

import (
	"fmt"
	"os"
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
	return buildExecutorConfig(c.Executor, c.Claude, c.Pi)
}

func buildExecutorConfig(name string, claudeCfg ClaudeConfig, piCfg PiConfig) llm.ExecutorConfig {
	cfg := llm.ExecutorConfig{Name: name}

	switch name {
	case "pi":
		cfg.Pi = llm.PiEnvConfig{
			ConfigDir: piCfg.ConfigDir,
			Provider:  piCfg.Provider,
			Model:     piCfg.Model,
			APIKey:    piCfg.APIKey,
		}
		cfg.ExtraFlags = strings.Fields(piCfg.Flags)
	default: // "claude" or ""
		configDir := claudeCfg.ConfigDir
		if envDir := os.Getenv("CLAUDE_CONFIG_DIR"); envDir != "" {
			configDir = envDir
		}
		cfg.Claude = llm.EnvConfig{
			ClaudeConfigDir: configDir,
			AnthropicAPIKey: claudeCfg.AnthropicAPIKey,
		}
		flags := strings.Fields(claudeCfg.Flags)
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

// toReviewExecutorConfig converts review-specific executor settings to llm.ExecutorConfig.
// It inherits top-level executor settings and applies review.executor overrides.
func (c *Config) toReviewExecutorConfig() llm.ExecutorConfig {
	name := c.Executor
	claudeCfg := c.Claude
	piCfg := c.Pi

	if c.Review.Executor.Name != "" {
		name = c.Review.Executor.Name
	}

	if c.Review.Executor.Claude.Flags != "" {
		claudeCfg.Flags = c.Review.Executor.Claude.Flags
	}
	if c.Review.Executor.Claude.ConfigDir != "" {
		claudeCfg.ConfigDir = c.Review.Executor.Claude.ConfigDir
	}
	if c.Review.Executor.Claude.AnthropicAPIKey != "" {
		claudeCfg.AnthropicAPIKey = c.Review.Executor.Claude.AnthropicAPIKey
	}
	if c.Review.Executor.Pi.Flags != "" {
		piCfg.Flags = c.Review.Executor.Pi.Flags
	}
	if c.Review.Executor.Pi.ConfigDir != "" {
		piCfg.ConfigDir = c.Review.Executor.Pi.ConfigDir
	}
	if c.Review.Executor.Pi.Provider != "" {
		piCfg.Provider = c.Review.Executor.Pi.Provider
	}
	if c.Review.Executor.Pi.Model != "" {
		piCfg.Model = c.Review.Executor.Pi.Model
	}
	if c.Review.Executor.Pi.APIKey != "" {
		piCfg.APIKey = c.Review.Executor.Pi.APIKey
	}

	return buildExecutorConfig(name, claudeCfg, piCfg)
}

func cloneAgentConfig(a review.AgentConfig) review.AgentConfig {
	out := a
	if a.Focus != nil {
		out.Focus = append([]string(nil), a.Focus...)
	}
	return out
}

func (c *Config) resolveReviewAgents() ([]review.AgentConfig, error) {
	if len(c.Review.Agents) > 0 {
		if len(c.Review.Include) > 0 || len(c.Review.Exclude) > 0 || len(c.Review.Overrides) > 0 {
			return nil, fmt.Errorf("review.agents cannot be combined with review.include/review.exclude/review.overrides")
		}

		custom := make([]review.AgentConfig, 0, len(c.Review.Agents))
		for _, agent := range c.Review.Agents {
			if agent.Name == "" {
				return nil, fmt.Errorf("review.agents contains entry with empty name")
			}
			if agent.Prompt != "" && agent.PromptFile != "" {
				return nil, fmt.Errorf("review.agents[%s]: prompt and prompt_file are mutually exclusive", agent.Name)
			}
			custom = append(custom, cloneAgentConfig(agent))
		}
		return custom, nil
	}

	defaults := review.DefaultAgents()
	byName := make(map[string]review.AgentConfig, len(defaults))
	for _, a := range defaults {
		byName[a.Name] = cloneAgentConfig(a)
	}

	selected := make([]review.AgentConfig, 0, len(defaults))
	if len(c.Review.Include) > 0 {
		seen := map[string]struct{}{}
		for _, name := range c.Review.Include {
			a, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("review.include references unknown default agent %q", name)
			}
			if _, dup := seen[name]; dup {
				return nil, fmt.Errorf("review.include contains duplicate agent %q", name)
			}
			seen[name] = struct{}{}
			selected = append(selected, cloneAgentConfig(a))
		}
	} else {
		for _, a := range defaults {
			selected = append(selected, cloneAgentConfig(a))
		}
	}

	if len(c.Review.Exclude) > 0 {
		excluded := map[string]struct{}{}
		for _, name := range c.Review.Exclude {
			if _, ok := byName[name]; !ok {
				return nil, fmt.Errorf("review.exclude references unknown default agent %q", name)
			}
			excluded[name] = struct{}{}
		}

		filtered := make([]review.AgentConfig, 0, len(selected))
		for _, a := range selected {
			if _, skip := excluded[a.Name]; skip {
				continue
			}
			filtered = append(filtered, a)
		}
		selected = filtered
	}

	if len(c.Review.Overrides) > 0 {
		index := make(map[string]int, len(selected))
		for i, a := range selected {
			index[a.Name] = i
		}

		for _, override := range c.Review.Overrides {
			if override.Name == "" {
				return nil, fmt.Errorf("review.overrides contains entry with empty name")
			}
			if override.Prompt != "" && override.PromptFile != "" {
				return nil, fmt.Errorf("review.overrides[%s]: prompt and prompt_file are mutually exclusive", override.Name)
			}

			i, ok := index[override.Name]
			if !ok {
				return nil, fmt.Errorf("review.overrides[%s] is not selected (check include/exclude)", override.Name)
			}

			merged := selected[i]
			if len(override.Focus) > 0 {
				merged.Focus = append([]string(nil), override.Focus...)
			}
			if override.Prompt != "" {
				merged.Prompt = override.Prompt
				merged.PromptFile = ""
			}
			if override.PromptFile != "" {
				merged.PromptFile = override.PromptFile
				merged.Prompt = ""
			}
			selected[i] = merged
		}
	}

	return selected, nil
}

// ToReviewConfig converts the unified Config to a review.Config.
func (c *Config) ToReviewConfig() (review.Config, error) {
	agents, err := c.resolveReviewAgents()
	if err != nil {
		return review.Config{}, err
	}

	return review.Config{
		MaxIterations:           c.Review.MaxIterations,
		Parallel:                c.Review.Parallel,
		Timeout:                 c.Timeout,
		Agents:                  agents,
		ExecutorConfig:          c.toReviewExecutorConfig(),
		ValidateIssues:          c.Review.Validators.Issue,
		ValidateSimplifications: c.Review.Validators.Simplification,
	}, nil
}
