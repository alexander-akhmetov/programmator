package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/review"
)

func TestToSafetyConfig(t *testing.T) {
	cfg := &Config{
		MaxIterations:   100,
		StagnationLimit: 5,
		Timeout:         600,
		ClaudeFlags:     "--verbose",
		ClaudeConfigDir: "/custom/dir",
		AnthropicAPIKey: "test-key",
		Review: ReviewConfig{
			MaxIterations: 10,
		},
	}

	sc := cfg.ToSafetyConfig()
	assert.Equal(t, 100, sc.MaxIterations)
	assert.Equal(t, 5, sc.StagnationLimit)
	assert.Equal(t, 600, sc.Timeout)
	assert.Equal(t, "--verbose", sc.ClaudeFlags)
	assert.Equal(t, "/custom/dir", sc.ClaudeConfigDir)
	assert.Equal(t, "test-key", sc.AnthropicAPIKey)
	assert.Equal(t, 10, sc.MaxReviewIterations)
}

func TestToReviewConfig(t *testing.T) {
	cfg := &Config{
		Timeout:     600,
		ClaudeFlags: "--verbose",
		Review: ReviewConfig{
			MaxIterations: 5,
			Phases: []ReviewPhase{
				{
					Name:           "test_phase",
					IterationLimit: 2,
					IterationPct:   25,
					Parallel:       true,
					Validate:       true,
					SeverityFilter: []string{"critical", "high"},
					Agents: []ReviewAgentConfig{
						{Name: "quality", Focus: []string{"bugs"}, Prompt: "custom.md"},
					},
				},
			},
		},
	}

	rc := cfg.ToReviewConfig()
	assert.Equal(t, 5, rc.MaxIterations)
	assert.Equal(t, 600, rc.Timeout)
	assert.Equal(t, "--verbose", rc.ClaudeFlags)
	assert.Len(t, rc.Phases, 1)

	phase := rc.Phases[0]
	assert.Equal(t, "test_phase", phase.Name)
	assert.Equal(t, 2, phase.IterationLimit)
	assert.Equal(t, 25, phase.IterationPct)
	assert.True(t, phase.Parallel)
	assert.True(t, phase.Validate)
	assert.Len(t, phase.SeverityFilter, 2)
	assert.Equal(t, review.Severity("critical"), phase.SeverityFilter[0])
	assert.Equal(t, review.Severity("high"), phase.SeverityFilter[1])
	assert.Len(t, phase.Agents, 1)
	assert.Equal(t, "quality", phase.Agents[0].Name)
	assert.Equal(t, "custom.md", phase.Agents[0].Prompt)
}

func TestToReviewConfig_EmptyPhases(t *testing.T) {
	cfg := &Config{
		Timeout:     300,
		ClaudeFlags: "",
		Review: ReviewConfig{
			MaxIterations: 3,
			Phases:        nil,
		},
	}

	rc := cfg.ToReviewConfig()
	assert.Equal(t, 3, rc.MaxIterations)
	assert.Empty(t, rc.Phases)
}

func TestToReviewConfig_EmptySeverityFilter(t *testing.T) {
	cfg := &Config{
		Review: ReviewConfig{
			MaxIterations: 1,
			Phases: []ReviewPhase{
				{
					Name:           "test",
					SeverityFilter: nil,
					Agents:         nil,
				},
			},
		},
	}

	rc := cfg.ToReviewConfig()
	require.Len(t, rc.Phases, 1)
	assert.Empty(t, rc.Phases[0].SeverityFilter)
	assert.Empty(t, rc.Phases[0].Agents)
}

func TestToSafetyConfig_ZeroValues(t *testing.T) {
	cfg := &Config{}
	sc := cfg.ToSafetyConfig()
	assert.Equal(t, 0, sc.MaxIterations)
	assert.Equal(t, 0, sc.StagnationLimit)
	assert.Equal(t, 0, sc.Timeout)
	assert.Equal(t, "", sc.ClaudeFlags)
}
