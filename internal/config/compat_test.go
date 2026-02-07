package config

import (
	"testing"

	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToSafetyConfig(t *testing.T) {
	cfg := &Config{
		MaxIterations:   100,
		StagnationLimit: 5,
		Timeout:         600,
		Executor:        "claude",
		Claude: ClaudeConfig{
			Flags:           "--verbose",
			ConfigDir:       "/custom/dir",
			AnthropicAPIKey: "test-key",
		},
		Review: ReviewConfig{
			MaxIterations: 10,
		},
	}

	sc := cfg.ToSafetyConfig()
	assert.Equal(t, 100, sc.MaxIterations)
	assert.Equal(t, 5, sc.StagnationLimit)
	assert.Equal(t, 600, sc.Timeout)
	assert.Equal(t, 10, sc.MaxReviewIterations)
}

func TestToExecutorConfig(t *testing.T) {
	cfg := &Config{
		Executor: "claude",
		Claude: ClaudeConfig{
			Flags:           "--verbose",
			ConfigDir:       "/custom/dir",
			AnthropicAPIKey: "test-key",
		},
	}

	ec := cfg.ToExecutorConfig()
	assert.Equal(t, "claude", ec.Name)
	assert.Equal(t, "--verbose", ec.ExtraFlags)
	assert.Equal(t, "/custom/dir", ec.Claude.ClaudeConfigDir)
	assert.Equal(t, "test-key", ec.Claude.AnthropicAPIKey)
}

func TestToReviewConfig_WithAgents(t *testing.T) {
	cfg := &Config{
		Timeout: 600,
		Claude: ClaudeConfig{
			Flags: "--verbose",
		},
		Review: ReviewConfig{
			MaxIterations: 5,
			Parallel:      true,
			Agents: []review.AgentConfig{
				{Name: "quality", Focus: []string{"bugs"}, Prompt: "custom.md"},
				{Name: "security", Focus: []string{"injection"}},
			},
		},
	}

	rc := cfg.ToReviewConfig()
	assert.Equal(t, 5, rc.MaxIterations)
	assert.Equal(t, 600, rc.Timeout)
	assert.Equal(t, "--verbose", rc.ClaudeFlags)
	assert.True(t, rc.Parallel)
	require.Len(t, rc.Agents, 2)
	assert.Equal(t, "quality", rc.Agents[0].Name)
	assert.Equal(t, "custom.md", rc.Agents[0].Prompt)
	assert.Equal(t, "security", rc.Agents[1].Name)
}

func TestToReviewConfig_MigrateFromPhases(t *testing.T) {
	cfg := &Config{
		Timeout: 300,
		Review: ReviewConfig{
			MaxIterations: 3,
			Phases: []ReviewPhase{
				{
					Name: "phase1",
					Agents: []review.AgentConfig{
						{Name: "quality", Focus: []string{"bugs"}},
						{Name: "security", Focus: []string{"injection"}},
					},
				},
				{
					Name: "phase2",
					Agents: []review.AgentConfig{
						{Name: "quality", Focus: []string{"different_focus"}}, // duplicate, should be deduped
						{Name: "implementation", Focus: []string{"completeness"}},
					},
				},
			},
		},
	}

	rc := cfg.ToReviewConfig()
	assert.Equal(t, 3, rc.MaxIterations)
	require.Len(t, rc.Agents, 3) // quality, security, implementation (deduped)
	assert.Equal(t, "quality", rc.Agents[0].Name)
	assert.Equal(t, "security", rc.Agents[1].Name)
	assert.Equal(t, "implementation", rc.Agents[2].Name)
}

func TestToReviewConfig_AgentsTakePrecedenceOverPhases(t *testing.T) {
	cfg := &Config{
		Review: ReviewConfig{
			MaxIterations: 5,
			Agents: []review.AgentConfig{
				{Name: "quality", Focus: []string{"bugs"}},
			},
			Phases: []ReviewPhase{
				{
					Name: "phase1",
					Agents: []review.AgentConfig{
						{Name: "security", Focus: []string{"injection"}},
					},
				},
			},
		},
	}

	rc := cfg.ToReviewConfig()
	require.Len(t, rc.Agents, 1)
	assert.Equal(t, "quality", rc.Agents[0].Name) // agents wins, not phases
}

func TestToReviewConfig_NoAgentsNoPhases(t *testing.T) {
	cfg := &Config{
		Timeout: 300,
		Review: ReviewConfig{
			MaxIterations: 3,
		},
	}

	rc := cfg.ToReviewConfig()
	assert.Equal(t, 3, rc.MaxIterations)
	assert.Empty(t, rc.Agents)
}

func TestToReviewConfig_CodexSettings(t *testing.T) {
	cfg := &Config{
		Codex: CodexConfig{
			Command:         "codex",
			Model:           "gpt-5.2-codex",
			ReasoningEffort: "xhigh",
			TimeoutMs:       3600000,
			Sandbox:         "read-only",
			ErrorPatterns:   []string{"Rate limit"},
		},
		Review: ReviewConfig{
			MaxIterations: 5,
		},
	}

	rc := cfg.ToReviewConfig()
	assert.Equal(t, "codex", rc.Codex.Command)
	assert.Equal(t, "gpt-5.2-codex", rc.Codex.Model)
	assert.Equal(t, "xhigh", rc.Codex.ReasoningEffort)
	assert.Equal(t, 3600000, rc.Codex.TimeoutMs)
	assert.Equal(t, "read-only", rc.Codex.Sandbox)
	assert.Equal(t, []string{"Rate limit"}, rc.Codex.ErrorPatterns)
}

func TestToSafetyConfig_ZeroValues(t *testing.T) {
	cfg := &Config{}
	sc := cfg.ToSafetyConfig()
	assert.Equal(t, 0, sc.MaxIterations)
	assert.Equal(t, 0, sc.StagnationLimit)
	assert.Equal(t, 0, sc.Timeout)
	assert.Equal(t, 0, sc.MaxReviewIterations)
}
