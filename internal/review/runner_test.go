package review

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunResult_HasCriticalIssues(t *testing.T) {
	t.Run("returns true for critical", func(t *testing.T) {
		result := &RunResult{
			Results: []*Result{
				{Issues: []Issue{{Severity: SeverityCritical}}},
			},
		}
		require.True(t, result.HasCriticalIssues())
	})

	t.Run("returns true for high", func(t *testing.T) {
		result := &RunResult{
			Results: []*Result{
				{Issues: []Issue{{Severity: SeverityHigh}}},
			},
		}
		require.True(t, result.HasCriticalIssues())
	})

	t.Run("returns false for medium", func(t *testing.T) {
		result := &RunResult{
			Results: []*Result{
				{Issues: []Issue{{Severity: SeverityMedium}}},
			},
		}
		require.False(t, result.HasCriticalIssues())
	})

	t.Run("returns false for no issues", func(t *testing.T) {
		result := &RunResult{
			Results: []*Result{
				{Issues: []Issue{}},
			},
		}
		require.False(t, result.HasCriticalIssues())
	})
}

func TestRunResult_AllIssues(t *testing.T) {
	result := &RunResult{
		Results: []*Result{
			{Issues: []Issue{{Description: "Issue 1"}, {Description: "Issue 2"}}},
			{Issues: []Issue{{Description: "Issue 3"}}},
		},
	}

	issues := result.AllIssues()
	require.Len(t, issues, 3)
}

func TestMockAgent(t *testing.T) {
	mock := NewMockAgent("test")
	require.Equal(t, "test", mock.Name())

	result, err := mock.Review(context.Background(), "/tmp", []string{})
	require.NoError(t, err)
	require.Equal(t, "test", result.AgentName)
	require.Empty(t, result.Issues)
	require.Equal(t, "Mock review passed", result.Summary)
}

func TestClaudeAgent(t *testing.T) {
	t.Run("constructs prompt correctly", func(t *testing.T) {
		agent := NewClaudeAgent("test", []string{"focus1", "focus2"}, "Base prompt")

		require.Equal(t, "test", agent.Name())

		// Test buildPrompt
		prompt := agent.buildPrompt([]string{"file1.go", "file2.go"})
		require.Contains(t, prompt, "Base prompt")
		require.Contains(t, prompt, "focus1")
		require.Contains(t, prompt, "focus2")
		require.Contains(t, prompt, "file1.go")
		require.Contains(t, prompt, "file2.go")
		require.Contains(t, prompt, "REVIEW_RESULT")
	})

	t.Run("respects options", func(t *testing.T) {
		agent := NewClaudeAgent(
			"test",
			nil,
			"prompt",
			WithTimeout(10*time.Minute),
			WithClaudeArgs([]string{"--verbose"}),
		)

		require.Equal(t, 10*time.Minute, agent.timeout)
		require.Equal(t, []string{"--verbose"}, agent.claudeArgs)
	})
}

func TestRunResult_FilterBySeverity(t *testing.T) {
	baseResult := &RunResult{
		Passed:    false,
		Iteration: 1,
		Results: []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{Severity: SeverityCritical, Description: "Critical issue"},
					{Severity: SeverityHigh, Description: "High issue"},
					{Severity: SeverityMedium, Description: "Medium issue"},
					{Severity: SeverityLow, Description: "Low issue"},
				},
			},
			{
				AgentName: "security",
				Issues: []Issue{
					{Severity: SeverityCritical, Description: "Another critical"},
					{Severity: SeverityInfo, Description: "Info issue"},
				},
			},
		},
		TotalIssues: 6,
	}

	t.Run("empty filter returns all", func(t *testing.T) {
		filtered := baseResult.FilterBySeverity([]Severity{})
		require.Equal(t, baseResult, filtered) // Same reference for passthrough
	})

	t.Run("filter by critical only", func(t *testing.T) {
		filtered := baseResult.FilterBySeverity([]Severity{SeverityCritical})
		require.Equal(t, 2, filtered.TotalIssues)
		require.False(t, filtered.Passed)

		// Check first agent's filtered issues
		require.Len(t, filtered.Results[0].Issues, 1)
		require.Equal(t, SeverityCritical, filtered.Results[0].Issues[0].Severity)

		// Check second agent's filtered issues
		require.Len(t, filtered.Results[1].Issues, 1)
		require.Equal(t, SeverityCritical, filtered.Results[1].Issues[0].Severity)
	})

	t.Run("filter by critical and high", func(t *testing.T) {
		filtered := baseResult.FilterBySeverity([]Severity{SeverityCritical, SeverityHigh})
		require.Equal(t, 3, filtered.TotalIssues)

		// First agent has 1 critical + 1 high
		require.Len(t, filtered.Results[0].Issues, 2)

		// Second agent has 1 critical
		require.Len(t, filtered.Results[1].Issues, 1)
	})

	t.Run("filter results in passed when no matching issues", func(t *testing.T) {
		smallResult := &RunResult{
			Passed:    false,
			Iteration: 1,
			Results: []*Result{
				{
					AgentName: "test",
					Issues: []Issue{
						{Severity: SeverityLow, Description: "Low issue"},
					},
				},
			},
			TotalIssues: 1,
		}

		filtered := smallResult.FilterBySeverity([]Severity{SeverityCritical})
		require.True(t, filtered.Passed)
		require.Equal(t, 0, filtered.TotalIssues)
	})
}

func TestRunner_RunPhase(t *testing.T) {
	t.Run("runs phase agents in parallel", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
		}

		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues:    []Issue{},
					Summary:   "No issues",
				}, nil
			})
			return mock
		})

		phase := Phase{
			Name:     "test_phase",
			Parallel: true,
			Agents: []AgentConfig{
				{Name: "agent1"},
				{Name: "agent2"},
			},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{"file.go"}, phase)
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, 0, result.TotalIssues)
		require.Len(t, result.Results, 2)
	})

	t.Run("runs phase agents sequentially when not parallel", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
		}

		callOrder := []string{}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				callOrder = append(callOrder, agentCfg.Name)
				return &Result{
					AgentName: agentCfg.Name,
					Issues:    []Issue{},
				}, nil
			})
			return mock
		})

		phase := Phase{
			Name:     "test_phase",
			Parallel: false,
			Agents: []AgentConfig{
				{Name: "first"},
				{Name: "second"},
			},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{}, phase)
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, []string{"first", "second"}, callOrder)
	})

	t.Run("counts issues correctly", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
		}

		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{Severity: SeverityHigh, Description: "Issue from " + agentCfg.Name},
					},
				}, nil
			})
			return mock
		})

		phase := Phase{
			Name:     "test_phase",
			Parallel: true,
			Agents: []AgentConfig{
				{Name: "agent1"},
				{Name: "agent2"},
			},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{}, phase)
		require.NoError(t, err)
		require.False(t, result.Passed)
		require.Equal(t, 2, result.TotalIssues)
	})
}
