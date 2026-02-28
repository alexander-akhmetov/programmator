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

func TestToExecutorConfig_Claude(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
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
	assert.Equal(t, []string{"--verbose", "--dangerously-skip-permissions"}, ec.ExtraFlags)
	assert.Equal(t, "/custom/dir", ec.Claude.ClaudeConfigDir)
	assert.Equal(t, "test-key", ec.Claude.AnthropicAPIKey)
}

func TestToExecutorConfig_Claude_YAMLConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/from/env")
	cfg := &Config{
		Executor: "claude",
		Claude: ClaudeConfig{
			ConfigDir: "/from/yaml",
		},
	}

	ec := cfg.ToExecutorConfig()
	assert.Equal(t, "/from/yaml", ec.Claude.ClaudeConfigDir)
}

func TestToExecutorConfig_Claude_NoConfigDir(t *testing.T) {
	cfg := &Config{Executor: "claude"}

	ec := cfg.ToExecutorConfig()
	assert.Empty(t, ec.Claude.ClaudeConfigDir)
}

func TestToExecutorConfig_Pi(t *testing.T) {
	cfg := &Config{
		Executor: "pi",
		Pi: PiConfig{
			Flags:     "--verbose",
			ConfigDir: "/custom/pi",
			Provider:  "anthropic",
			Model:     "sonnet",
			APIKey:    "pi-key",
		},
	}

	ec := cfg.ToExecutorConfig()
	assert.Equal(t, "pi", ec.Name)
	assert.Equal(t, "/custom/pi", ec.Pi.ConfigDir)
	assert.Equal(t, "anthropic", ec.Pi.Provider)
	assert.Equal(t, "sonnet", ec.Pi.Model)
	assert.Equal(t, "pi-key", ec.Pi.APIKey)
	assert.Equal(t, []string{"--verbose"}, ec.ExtraFlags)
	assert.NotContains(t, ec.ExtraFlags, "--dangerously-skip-permissions")
}

func TestToExecutorConfig_AlwaysSkipPermissions(t *testing.T) {
	t.Run("injects flag when absent", func(t *testing.T) {
		cfg := &Config{}
		ec := cfg.ToExecutorConfig()
		assert.Contains(t, ec.ExtraFlags, "--dangerously-skip-permissions")
	})

	t.Run("does not duplicate when already present", func(t *testing.T) {
		cfg := &Config{Claude: ClaudeConfig{Flags: "--dangerously-skip-permissions"}}
		ec := cfg.ToExecutorConfig()
		count := 0
		for _, f := range ec.ExtraFlags {
			if f == "--dangerously-skip-permissions" {
				count++
			}
		}
		assert.Equal(t, 1, count)
	})
}

func TestToExecutorConfig_OpenCode(t *testing.T) {
	cfg := &Config{
		Executor: "opencode",
		OpenCode: OpenCodeConfig{
			Flags:     "--verbose",
			ConfigDir: "/custom/opencode",
			Model:     "anthropic/claude-sonnet-4-5",
			APIKey:    "oc-key",
		},
	}

	ec := cfg.ToExecutorConfig()
	assert.Equal(t, "opencode", ec.Name)
	assert.Equal(t, "/custom/opencode", ec.OpenCode.ConfigDir)
	assert.Equal(t, "anthropic/claude-sonnet-4-5", ec.OpenCode.Model)
	assert.Equal(t, "oc-key", ec.OpenCode.APIKey)
	assert.Equal(t, []string{"--verbose"}, ec.ExtraFlags)
	assert.NotContains(t, ec.ExtraFlags, "--dangerously-skip-permissions")
}

func TestToReviewConfig_UsesReviewExecutorOpenCode(t *testing.T) {
	cfg := &Config{
		Executor: "claude",
		Review: ReviewConfig{
			Executor: ReviewExecutorConfig{
				Name: "opencode",
				OpenCode: OpenCodeConfig{
					Model: "openai/gpt-4o",
				},
			},
		},
	}

	rc, err := cfg.ToReviewConfig()
	require.NoError(t, err)
	assert.Equal(t, "opencode", rc.ExecutorConfig.Name)
	assert.Equal(t, "openai/gpt-4o", rc.ExecutorConfig.OpenCode.Model)
}

func TestToReviewConfig_WithCustomAgents(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	cfg := &Config{
		Executor: "claude",
		Timeout:  600,
		Claude: ClaudeConfig{
			Flags:           "--verbose",
			ConfigDir:       "/custom/config",
			AnthropicAPIKey: "test-api-key",
		},
		Review: ReviewConfig{
			MaxIterations: 5,
			Parallel:      true,
			Agents: []review.AgentConfig{
				{Name: "my-review", Focus: []string{"custom"}, Prompt: "inline prompt"},
			},
			Validators: ReviewValidatorsConfig{
				Issue:          false,
				Simplification: true,
			},
		},
	}

	rc, err := cfg.ToReviewConfig()
	require.NoError(t, err)
	assert.Equal(t, 5, rc.MaxIterations)
	assert.Equal(t, 600, rc.Timeout)
	assert.True(t, rc.Parallel)
	assert.False(t, rc.ValidateIssues)
	assert.True(t, rc.ValidateSimplifications)
	require.Len(t, rc.Agents, 1)
	assert.Equal(t, "my-review", rc.Agents[0].Name)
	assert.Equal(t, "inline prompt", rc.Agents[0].Prompt)
	assert.Equal(t, "/custom/config", rc.ExecutorConfig.Claude.ClaudeConfigDir)
	assert.Equal(t, "test-api-key", rc.ExecutorConfig.Claude.AnthropicAPIKey)
	assert.Contains(t, rc.ExecutorConfig.ExtraFlags, "--dangerously-skip-permissions")
}

func TestToReviewConfig_DefaultAgentsSelectedByIncludeExclude(t *testing.T) {
	cfg := &Config{
		Review: ReviewConfig{
			Include: []string{"bug-shallow", "bug-deep", "architect"},
			Exclude: []string{"architect"},
		},
	}

	rc, err := cfg.ToReviewConfig()
	require.NoError(t, err)
	require.Len(t, rc.Agents, 2)
	assert.Equal(t, "bug-shallow", rc.Agents[0].Name)
	assert.Equal(t, "bug-deep", rc.Agents[1].Name)
}

func TestToReviewConfig_DefaultAgentOverride(t *testing.T) {
	cfg := &Config{
		Review: ReviewConfig{
			Overrides: []review.AgentConfig{
				{
					Name:       "bug-deep",
					PromptFile: "my_prompt.md",
				},
			},
		},
	}

	rc, err := cfg.ToReviewConfig()
	require.NoError(t, err)

	var found *review.AgentConfig
	for i := range rc.Agents {
		if rc.Agents[i].Name == "bug-deep" {
			found = &rc.Agents[i]
			break
		}
	}
	require.NotNil(t, found)
	assert.Equal(t, "my_prompt.md", found.PromptFile)
}

func TestToReviewConfig_UsesReviewExecutorOverride(t *testing.T) {
	cfg := &Config{
		Executor: "pi",
		Pi: PiConfig{
			Provider: "anthropic",
			Model:    "sonnet",
		},
		Review: ReviewConfig{
			Executor: ReviewExecutorConfig{
				Name: "claude",
				Claude: ClaudeConfig{
					Flags: "--model opus",
				},
			},
		},
	}

	rc, err := cfg.ToReviewConfig()
	require.NoError(t, err)
	assert.Equal(t, "claude", rc.ExecutorConfig.Name)
	assert.Contains(t, rc.ExecutorConfig.ExtraFlags, "--model")
	assert.Contains(t, rc.ExecutorConfig.ExtraFlags, "opus")
	assert.Contains(t, rc.ExecutorConfig.ExtraFlags, "--dangerously-skip-permissions")
}

func TestToReviewConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{
			name: "rejects mixed custom and default selectors",
			cfg: &Config{
				Review: ReviewConfig{
					Agents:  []review.AgentConfig{{Name: "custom"}},
					Include: []string{"bug-shallow"},
				},
			},
			wantErr: "cannot be combined",
		},
		{
			name: "rejects unknown include",
			cfg: &Config{
				Review: ReviewConfig{
					Include: []string{"not-a-real-agent"},
				},
			},
			wantErr: "unknown default agent",
		},
		{
			name: "rejects prompt and prompt_file together",
			cfg: &Config{
				Review: ReviewConfig{
					Agents: []review.AgentConfig{
						{
							Name:       "custom",
							Prompt:     "inline",
							PromptFile: "file.md",
						},
					},
				},
			},
			wantErr: "mutually exclusive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.cfg.ToReviewConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
