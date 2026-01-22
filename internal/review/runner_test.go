package review

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunner_Run(t *testing.T) {
	t.Run("passes when no issues", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
			Passes: []Pass{
				{
					Name:     "test_pass",
					Parallel: true,
					Agents: []AgentConfig{
						{Name: "agent1"},
					},
				},
			},
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

		result, err := runner.Run(context.Background(), "/tmp", []string{"file.go"})
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, 0, result.TotalIssues)
	})

	t.Run("fails when issues found", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
			Passes: []Pass{
				{
					Name:     "test_pass",
					Parallel: true,
					Agents: []AgentConfig{
						{Name: "agent1"},
					},
				},
			},
		}

		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{File: "file.go", Severity: SeverityHigh, Description: "Problem"},
					},
					Summary: "Found issues",
				}, nil
			})
			return mock
		})

		result, err := runner.Run(context.Background(), "/tmp", []string{"file.go"})
		require.NoError(t, err)
		require.False(t, result.Passed)
		require.Equal(t, 1, result.TotalIssues)
	})

	t.Run("handles agent error", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
			Passes: []Pass{
				{
					Name:     "test_pass",
					Parallel: false,
					Agents: []AgentConfig{
						{Name: "agent1"},
					},
				},
			},
		}

		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return nil, errors.New("agent error")
			})
			return mock
		})

		result, err := runner.Run(context.Background(), "/tmp", []string{"file.go"})
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		require.NotNil(t, result.Results[0].Error)
	})

	t.Run("runs multiple passes", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
			Passes: []Pass{
				{
					Name:   "pass1",
					Agents: []AgentConfig{{Name: "agent1"}},
				},
				{
					Name:   "pass2",
					Agents: []AgentConfig{{Name: "agent2"}},
				},
			},
		}

		agentsCalled := make(map[string]bool)
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				agentsCalled[agentCfg.Name] = true
				return &Result{AgentName: agentCfg.Name, Issues: []Issue{}}, nil
			})
			return mock
		})

		result, err := runner.Run(context.Background(), "/tmp", []string{})
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.True(t, agentsCalled["agent1"])
		require.True(t, agentsCalled["agent2"])
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		cfg := Config{
			Enabled:       true,
			MaxIterations: 3,
			Passes: []Pass{
				{
					Name:     "test_pass",
					Parallel: false,
					Agents: []AgentConfig{
						{Name: "agent1"},
						{Name: "agent2"},
					},
				},
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		callCount := 0

		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				callCount++
				if callCount == 1 {
					cancel() // Cancel after first agent
				}
				return &Result{AgentName: agentCfg.Name, Issues: []Issue{}}, nil
			})
			return mock
		})

		_, err := runner.Run(ctx, "/tmp", []string{})
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	})
}

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

func TestRunner_OutputCallback(t *testing.T) {
	cfg := Config{
		Enabled:       true,
		MaxIterations: 1,
		Passes: []Pass{
			{
				Name:   "test_pass",
				Agents: []AgentConfig{{Name: "agent1"}},
			},
		},
	}

	var logs []string
	runner := NewRunner(cfg, func(text string) {
		logs = append(logs, text)
	})
	runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
		return NewMockAgent(agentCfg.Name)
	})

	_, err := runner.Run(context.Background(), "/tmp", []string{})
	require.NoError(t, err)
	require.NotEmpty(t, logs)
	require.Contains(t, logs[0], "[REVIEW]")
}

func TestRunner_RegisterAgent(t *testing.T) {
	cfg := Config{
		Enabled:       true,
		MaxIterations: 1,
		Passes: []Pass{
			{
				Name:   "test_pass",
				Agents: []AgentConfig{{Name: "custom"}},
			},
		},
	}

	customAgent := NewMockAgent("custom")
	customAgent.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
		return &Result{
			AgentName: "custom",
			Issues:    []Issue{{Description: "Custom issue"}},
		}, nil
	})

	runner := NewRunner(cfg, nil)
	runner.RegisterAgent(customAgent)

	result, err := runner.Run(context.Background(), "/tmp", []string{})
	require.NoError(t, err)
	require.Equal(t, 1, result.TotalIssues)
	require.Equal(t, "Custom issue", result.Results[0].Issues[0].Description)
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
