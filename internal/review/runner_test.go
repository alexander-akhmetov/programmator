package review

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/worksonmyai/programmator/internal/protocol"
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
		require.Contains(t, prompt, protocol.ReviewResultBlockKey)
	})

	t.Run("respects options", func(t *testing.T) {
		agent := NewClaudeAgent(
			"test",
			nil,
			"prompt",
			WithTimeout(10*time.Minute),
			WithClaudeArgs([]string{"--verbose"}),
			WithSettingsJSON(`{"hooks":{}}`),
		)

		require.Equal(t, 10*time.Minute, agent.timeout)
		require.Equal(t, []string{"--verbose"}, agent.claudeArgs)
		require.Equal(t, `{"hooks":{}}`, agent.settingsJSON)
	})
}

func TestDefaultAgentFactory_PassesClaudeFlagsAndSettings(t *testing.T) {
	cfg := Config{
		MaxIterations: 3,
		Timeout:       120,
		ClaudeFlags:   "--dangerously-skip-permissions",
		SettingsJSON:  `{"hooks":{"PreToolUse":[]}}`,
	}

	runner := NewRunner(cfg, nil)
	agent := runner.defaultAgentFactory(AgentConfig{Name: "test", Focus: []string{"bugs"}}, "default prompt")

	claudeAgent, ok := agent.(*ClaudeAgent)
	require.True(t, ok)
	require.Equal(t, []string{"--dangerously-skip-permissions"}, claudeAgent.claudeArgs)
	require.Equal(t, `{"hooks":{"PreToolUse":[]}}`, claudeAgent.settingsJSON)
	require.Equal(t, 120*time.Second, claudeAgent.timeout)
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

func TestIssueFingerprint(t *testing.T) {
	t.Run("deterministic across calls", func(t *testing.T) {
		issue := Issue{
			File:        "main.go",
			Line:        42,
			Severity:    SeverityHigh,
			Category:    "error handling",
			Description: "Error is ignored",
		}
		id1 := issueFingerprint("quality", issue)
		id2 := issueFingerprint("quality", issue)
		require.Equal(t, id1, id2)
		require.Len(t, id1, 16) // 8 bytes = 16 hex chars
	})

	t.Run("different agents produce different IDs", func(t *testing.T) {
		issue := Issue{
			File:        "main.go",
			Category:    "bugs",
			Description: "Some bug",
		}
		id1 := issueFingerprint("quality", issue)
		id2 := issueFingerprint("security", issue)
		require.NotEqual(t, id1, id2)
	})

	t.Run("different files produce different IDs", func(t *testing.T) {
		issue1 := Issue{File: "a.go", Category: "bugs", Description: "Bug"}
		issue2 := Issue{File: "b.go", Category: "bugs", Description: "Bug"}
		require.NotEqual(t, issueFingerprint("agent", issue1), issueFingerprint("agent", issue2))
	})

	t.Run("description is case-insensitive", func(t *testing.T) {
		issue1 := Issue{File: "a.go", Category: "bugs", Description: "Error Ignored"}
		issue2 := Issue{File: "a.go", Category: "bugs", Description: "error ignored"}
		require.Equal(t, issueFingerprint("agent", issue1), issueFingerprint("agent", issue2))
	})

	t.Run("description whitespace is trimmed", func(t *testing.T) {
		issue1 := Issue{File: "a.go", Category: "bugs", Description: "  Error ignored  "}
		issue2 := Issue{File: "a.go", Category: "bugs", Description: "error ignored"}
		require.Equal(t, issueFingerprint("agent", issue1), issueFingerprint("agent", issue2))
	})

	t.Run("different lines produce different IDs", func(t *testing.T) {
		issue1 := Issue{File: "a.go", Line: 10, Category: "bugs", Description: "Bug"}
		issue2 := Issue{File: "a.go", Line: 20, Category: "bugs", Description: "Bug"}
		require.NotEqual(t, issueFingerprint("agent", issue1), issueFingerprint("agent", issue2))
	})

	t.Run("category is case-insensitive", func(t *testing.T) {
		issue1 := Issue{File: "a.go", Category: "Bugs", Description: "Bug"}
		issue2 := Issue{File: "a.go", Category: "bugs", Description: "Bug"}
		require.Equal(t, issueFingerprint("agent", issue1), issueFingerprint("agent", issue2))
	})

	t.Run("empty fields produce deterministic ID", func(t *testing.T) {
		issue := Issue{}
		id1 := issueFingerprint("", issue)
		id2 := issueFingerprint("", issue)
		require.Equal(t, id1, id2)
		require.Len(t, id1, 16)
	})
}

func TestAssignIssueIDs(t *testing.T) {
	t.Run("assigns IDs to issues without them", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{File: "a.go", Category: "bugs", Description: "Bug 1"},
					{File: "b.go", Category: "bugs", Description: "Bug 2"},
				},
			},
		}

		assignIssueIDs(results)

		require.NotEmpty(t, results[0].Issues[0].ID)
		require.NotEmpty(t, results[0].Issues[1].ID)
		require.NotEqual(t, results[0].Issues[0].ID, results[0].Issues[1].ID)
	})

	t.Run("preserves existing IDs", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "existing-id", File: "a.go", Category: "bugs", Description: "Bug"},
				},
			},
		}

		assignIssueIDs(results)

		require.Equal(t, "existing-id", results[0].Issues[0].ID)
	})

	t.Run("handles empty results", func(t *testing.T) {
		results := []*Result{}
		assignIssueIDs(results)
		require.Empty(t, results)
	})

	t.Run("handles results with no issues", func(t *testing.T) {
		results := []*Result{
			{AgentName: "quality", Issues: []Issue{}},
		}
		assignIssueIDs(results)
		require.Empty(t, results[0].Issues)
	})
}

func TestRunner_ValidateSimplifications(t *testing.T) {
	t.Run("filters simplification issues through validator", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{Severity: SeverityMedium, Description: "Kept suggestion", File: "a.go", Line: 1},
					},
					Summary: "Filtered",
				}, nil
			})
			return mock
		})

		original := &Result{
			AgentName: "simplification",
			Issues: []Issue{
				{Severity: SeverityMedium, Description: "Minor nitpick", File: "a.go", Line: 1},
				{Severity: SeverityMedium, Description: "Real simplification", File: "b.go", Line: 5},
			},
			Summary: "2 suggestions",
		}

		validated, err := runner.ValidateSimplifications(context.Background(), "/tmp", original)
		require.NoError(t, err)
		require.Equal(t, "simplification", validated.AgentName)
		require.Len(t, validated.Issues, 1)
	})

	t.Run("returns original on empty issues", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		original := &Result{
			AgentName: "simplification",
			Issues:    []Issue{},
			Summary:   "No issues",
		}

		validated, err := runner.ValidateSimplifications(context.Background(), "/tmp", original)
		require.NoError(t, err)
		require.Equal(t, original, validated)
	})

	t.Run("handles nil result from validator", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return nil, nil
			})
			return mock
		})

		original := &Result{
			AgentName: "simplification",
			Issues: []Issue{
				{Severity: SeverityMedium, Description: "Some suggestion"},
			},
			Summary: "1 suggestion",
		}

		validated, err := runner.ValidateSimplifications(context.Background(), "/tmp", original)
		require.NoError(t, err)
		require.Equal(t, "simplification", validated.AgentName)
		require.Empty(t, validated.Issues)
		require.Equal(t, "All simplification suggestions filtered by validator", validated.Summary)
	})

	t.Run("falls back to original on validator error", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return nil, fmt.Errorf("validator failed")
			})
			return mock
		})

		original := &Result{
			AgentName: "simplification",
			Issues: []Issue{
				{Severity: SeverityMedium, Description: "Some suggestion"},
			},
			Summary: "1 suggestion",
		}

		validated, err := runner.ValidateSimplifications(context.Background(), "/tmp", original)
		require.NoError(t, err)
		require.Equal(t, original, validated)
	})
}

func TestRunner_RunPhase(t *testing.T) {
	t.Run("runs phase agents in parallel", func(t *testing.T) {
		cfg := Config{
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

	t.Run("assigns IDs to issues", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug found"},
					},
				}, nil
			})
			return mock
		})

		phase := Phase{
			Name:     "test_phase",
			Parallel: true,
			Agents:   []AgentConfig{{Name: "agent1"}},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{"a.go"}, phase)
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		require.NotEmpty(t, result.Results[0].Issues[0].ID)
	})

	t.Run("empty agents list passes", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		phase := Phase{
			Name:   "empty_phase",
			Agents: []AgentConfig{},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{}, phase)
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, 0, result.TotalIssues)
		require.Empty(t, result.Results)
	})

	t.Run("filters issues via validator", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		// The quality agent returns 2 issues; the validator confirms only 1 by ID
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			switch agentCfg.Name {
			case "quality":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "quality",
						Issues: []Issue{
							{ID: "id-keep", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Real bug"},
							{ID: "id-drop", File: "b.go", Severity: SeverityLow, Category: "style", Description: "False positive"},
						},
					}, nil
				})
			case "issue-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "issue-validator",
						Issues: []Issue{
							{ID: "id-keep", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Real bug"},
						},
						Summary: "Validated 1 of 2",
					}, nil
				})
			}
			return mock
		})

		phase := Phase{
			Name:     "test_validate",
			Parallel: false,
			Agents:   []AgentConfig{{Name: "quality"}},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{"a.go"}, phase)
		require.NoError(t, err)
		require.Equal(t, 1, result.TotalIssues)
		require.Len(t, result.Results[0].Issues, 1)
		require.Equal(t, "id-keep", result.Results[0].Issues[0].ID)
		for _, issue := range result.Results[0].Issues {
			require.NotEqual(t, "id-drop", issue.ID)
		}
	})
	t.Run("validator runs by default", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		validatorCalled := false
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			switch agentCfg.Name {
			case "quality":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "quality",
						Issues: []Issue{
							{File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
						},
					}, nil
				})
			case "issue-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					validatorCalled = true
					return &Result{AgentName: "issue-validator", Issues: []Issue{}}, nil
				})
			}
			return mock
		})

		phase := Phase{
			Name:     "test_validate_default",
			Parallel: false,
			Agents:   []AgentConfig{{Name: "quality"}},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{"a.go"}, phase)
		require.NoError(t, err)
		require.True(t, validatorCalled)
		require.Equal(t, 0, result.TotalIssues)
	})

	t.Run("filters using generated IDs", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			switch agentCfg.Name {
			case "quality":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "quality",
						Issues: []Issue{
							{File: "a.go", Line: 10, Severity: SeverityHigh, Category: "bugs", Description: "Real bug"},
							{File: "b.go", Line: 20, Severity: SeverityLow, Category: "style", Description: "False positive"},
						},
					}, nil
				})
			case "issue-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, filesChanged []string) (*Result, error) {
					// Parse the YAML from the VALIDATION_INPUT to extract generated IDs
					var yamlContent string
					for _, f := range filesChanged {
						if _, after, ok := strings.Cut(f, "VALIDATION_INPUT:\n"); ok {
							yamlContent = after
							break
						}
					}

					type validatorIssue struct {
						ID   string `yaml:"id"`
						File string `yaml:"file"`
					}
					var parsed struct {
						Issues []validatorIssue `yaml:"issues"`
					}
					if err := yaml.Unmarshal([]byte(yamlContent), &parsed); err == nil && len(parsed.Issues) > 0 {
						// Confirm only the first issue (a.go)
						for _, iss := range parsed.Issues {
							if iss.File == "a.go" {
								return &Result{
									AgentName: "issue-validator",
									Issues:    []Issue{{ID: iss.ID}},
									Summary:   "Validated 1 of 2",
								}, nil
							}
						}
					}
					return &Result{AgentName: "issue-validator", Issues: []Issue{}}, nil
				})
			}
			return mock
		})

		phase := Phase{
			Name:     "test_validate_generated",
			Parallel: false,
			Agents:   []AgentConfig{{Name: "quality"}},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{"a.go", "b.go"}, phase)
		require.NoError(t, err)
		require.Equal(t, 1, result.TotalIssues)
		require.Len(t, result.Results[0].Issues, 1)
		require.Equal(t, "Real bug", result.Results[0].Issues[0].Description)
		require.NotEmpty(t, result.Results[0].Issues[0].ID)
	})

	t.Run("both simplification and issue validation run independently", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		simpValidatorCalled := false
		issueValidatorCalled := false

		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			switch agentCfg.Name {
			case "quality":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "quality",
						Issues: []Issue{
							{File: "a.go", Line: 10, Severity: SeverityHigh, Category: "bugs", Description: "Real bug"},
							{File: "b.go", Line: 20, Severity: SeverityLow, Category: "style", Description: "FP"},
						},
					}, nil
				})
			case "simplification":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "simplification",
						Issues: []Issue{
							{File: "c.go", Line: 30, Severity: SeverityMedium, Category: "simplification", Description: "Good simplification"},
							{File: "d.go", Line: 40, Severity: SeverityLow, Category: "simplification", Description: "Bad simplification"},
						},
					}, nil
				})
			case "simplification-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					simpValidatorCalled = true
					return &Result{
						AgentName: "simplification-validator",
						Issues: []Issue{
							{File: "c.go", Line: 30, Severity: SeverityMedium, Description: "Good simplification"},
						},
					}, nil
				})
			case "issue-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, filesChanged []string) (*Result, error) {
					issueValidatorCalled = true
					// Parse the YAML to get IDs and confirm only the real bug
					var yamlContent string
					for _, f := range filesChanged {
						if _, after, ok := strings.Cut(f, "VALIDATION_INPUT:\n"); ok {
							yamlContent = after
							break
						}
					}
					type vi struct {
						ID   string `yaml:"id"`
						File string `yaml:"file"`
					}
					var parsed struct {
						Issues []vi `yaml:"issues"`
					}
					if err := yaml.Unmarshal([]byte(yamlContent), &parsed); err == nil {
						for _, iss := range parsed.Issues {
							if iss.File == "a.go" {
								return &Result{
									AgentName: "issue-validator",
									Issues:    []Issue{{ID: iss.ID}},
								}, nil
							}
						}
					}
					return &Result{AgentName: "issue-validator", Issues: []Issue{}}, nil
				})
			}
			return mock
		})

		phase := Phase{
			Name:     "test_both_validators",
			Parallel: false,
			Agents: []AgentConfig{
				{Name: "quality"},
				{Name: "simplification"},
			},
		}

		result, err := runner.RunPhase(context.Background(), "/tmp", []string{"a.go", "b.go", "c.go", "d.go"}, phase)
		require.NoError(t, err)
		require.True(t, simpValidatorCalled)
		require.True(t, issueValidatorCalled)

		// Quality: only "Real bug" kept (FP filtered by issue-validator)
		var qualityResult *Result
		var simpResult *Result
		for _, res := range result.Results {
			switch res.AgentName {
			case "quality":
				qualityResult = res
			case "simplification":
				simpResult = res
			}
		}
		require.NotNil(t, qualityResult)
		require.Len(t, qualityResult.Issues, 1)
		require.Equal(t, "Real bug", qualityResult.Issues[0].Description)

		// Simplification: only "Good simplification" kept
		require.NotNil(t, simpResult)
		require.Len(t, simpResult.Issues, 1)
		require.Equal(t, "Good simplification", simpResult.Issues[0].Description)

		// Total: 1 quality + 1 simplification = 2
		require.Equal(t, 2, result.TotalIssues)
	})
}

func TestRunner_ValidateIssues(t *testing.T) {
	t.Run("filters issues by validator output", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "confirmed-1", File: "x.go", Severity: SeverityHigh, Category: "bugs", Description: "Confirmed"},
					},
					Summary: "1 confirmed",
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "confirmed-1", File: "x.go", Severity: SeverityHigh, Category: "bugs", Description: "Confirmed"},
					{ID: "filtered-out", File: "y.go", Severity: SeverityLow, Category: "style", Description: "FP"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Len(t, validated, 1)
		require.Len(t, validated[0].Issues, 1)
		require.Equal(t, "confirmed-1", validated[0].Issues[0].ID)
	})

	t.Run("excludes simplification from validation", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		validatorCalled := false
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			if agentCfg.Name == "issue-validator" {
				mock.SetReviewFunc(func(_ context.Context, _ string, filesChanged []string) (*Result, error) {
					validatorCalled = true
					// Validator should not see simplification issues
					for _, f := range filesChanged {
						require.NotContains(t, f, "simplification-issue")
					}
					return &Result{
						AgentName: "issue-validator",
						Issues: []Issue{
							{ID: "quality-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
						},
					}, nil
				})
			}
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "quality-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
				},
			},
			{
				AgentName: "simplification",
				Issues: []Issue{
					{ID: "simplification-issue", File: "b.go", Severity: SeverityMedium, Category: "simplification", Description: "Simplify"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.True(t, validatorCalled)
		// Simplification result should pass through unchanged
		require.Len(t, validated[1].Issues, 1)
		require.Equal(t, "simplification", validated[1].AgentName)
		require.Equal(t, "simplification-issue", validated[1].Issues[0].ID)
		// Quality result should be filtered
		require.Len(t, validated[0].Issues, 1)
		require.Equal(t, "quality-1", validated[0].Issues[0].ID)
	})

	t.Run("fallback on error returns original", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return nil, fmt.Errorf("validator crashed")
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Equal(t, input, validated)
	})

	t.Run("nil validator result keeps original results unchanged", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return nil, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		// nil result with no error falls back to original results unchanged
		require.Equal(t, input, validated)
	})

	t.Run("missing structured output summary keeps original results unchanged", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues:    []Issue{},
					Summary:   noStructuredReviewOutputSummary,
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		// Missing structured output should fall back to original results unchanged
		require.Equal(t, input, validated)
	})

	t.Run("orphan IDs from validator are ignored", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "confirmed-1", File: "x.go", Severity: SeverityHigh, Description: "Real"},
						{ID: "orphan-id", File: "z.go", Severity: SeverityHigh, Description: "Orphan"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "confirmed-1", File: "x.go", Severity: SeverityHigh, Category: "bugs", Description: "Real"},
					{ID: "will-drop", File: "y.go", Severity: SeverityLow, Category: "style", Description: "Drop"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Len(t, validated[0].Issues, 1)
		require.Equal(t, "confirmed-1", validated[0].Issues[0].ID)
		// Original fields preserved
		require.Equal(t, "bugs", validated[0].Issues[0].Category)
		require.Equal(t, SeverityHigh, validated[0].Issues[0].Severity)
	})

	t.Run("preserves original issue data when filtering", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", File: "x.go", Severity: SeverityMedium, Description: "Modified description"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "x.go", Severity: SeverityHigh, Category: "bugs", Description: "Original description", Suggestion: "Fix it"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Len(t, validated[0].Issues, 1)
		// Original data preserved, not validator's modified fields
		require.Equal(t, SeverityHigh, validated[0].Issues[0].Severity)
		require.Equal(t, "Original description", validated[0].Issues[0].Description)
		require.Equal(t, "Fix it", validated[0].Issues[0].Suggestion)
	})

	t.Run("skips when no non-simplification issues", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		validatorCalled := false
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			if agentCfg.Name == "issue-validator" {
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					validatorCalled = true
					return &Result{AgentName: "issue-validator"}, nil
				})
			}
			return mock
		})

		input := []*Result{
			{AgentName: "quality", Issues: []Issue{}},
			{
				AgentName: "simplification",
				Issues: []Issue{
					{ID: "s-1", File: "a.go", Severity: SeverityMedium, Category: "simplification", Description: "Simplify"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.False(t, validatorCalled)
		require.Equal(t, input, validated)
	})

	t.Run("context cancellation returns original results", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(ctx context.Context, _ string, _ []string) (*Result, error) {
				return nil, ctx.Err()
			})
			return mock
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
				},
			},
		}

		validated, err := runner.ValidateIssues(ctx, "/tmp", input)
		require.NoError(t, err)
		require.Equal(t, input, validated)
	})

	t.Run("validator returning empty issues filters all", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues:    []Issue{},
					Summary:   "All false positives",
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
					{ID: "id-2", File: "b.go", Severity: SeverityMedium, Category: "style", Description: "Style"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Len(t, validated[0].Issues, 0)
	})

	t.Run("validator returning issues without IDs falls back to originals", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{File: "a.go", Severity: SeverityHigh, Description: "No ID here"},
					},
					Summary: "Found issues but no IDs",
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Equal(t, input, validated)
	})

	t.Run("issues without IDs in input are skipped as defensive fallback", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
					{ID: "", File: "b.go", Severity: SeverityLow, Category: "style", Description: "No ID issue"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Len(t, validated[0].Issues, 1)
		require.Equal(t, "id-1", validated[0].Issues[0].ID)
	})

	t.Run("validator with mixed IDs uses only those with IDs for filtering", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", File: "a.go", Severity: SeverityHigh, Description: "Confirmed with ID"},
						{File: "b.go", Severity: SeverityMedium, Description: "No ID from validator"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug 1"},
					{ID: "id-2", File: "c.go", Severity: SeverityMedium, Category: "style", Description: "Bug 2"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		// Only id-1 confirmed; id-2 not in validator output so filtered out
		require.Len(t, validated[0].Issues, 1)
		require.Equal(t, "id-1", validated[0].Issues[0].ID)
	})

	t.Run("filters across multiple non-simplification agents", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "q-1", File: "a.go", Severity: SeverityHigh, Description: "Quality confirmed"},
						{ID: "s-1", File: "b.go", Severity: SeverityCritical, Description: "Security confirmed"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "q-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Quality confirmed"},
					{ID: "q-2", File: "c.go", Severity: SeverityLow, Category: "style", Description: "Quality FP"},
				},
			},
			{
				AgentName: "security",
				Issues: []Issue{
					{ID: "s-1", File: "b.go", Severity: SeverityCritical, Category: "injection", Description: "Security confirmed"},
					{ID: "s-2", File: "d.go", Severity: SeverityMedium, Category: "auth", Description: "Security FP"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.Len(t, validated, 2)
		require.Len(t, validated[0].Issues, 1)
		require.Equal(t, "q-1", validated[0].Issues[0].ID)
		require.Len(t, validated[1].Issues, 1)
		require.Equal(t, "s-1", validated[1].Issues[0].ID)
	})

	t.Run("skips when only simplification agents present", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)

		validatorCalled := false
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			if agentCfg.Name == "issue-validator" {
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					validatorCalled = true
					return &Result{AgentName: "issue-validator"}, nil
				})
			}
			return mock
		})

		input := []*Result{
			{
				AgentName: "simplification",
				Issues: []Issue{
					{ID: "s-1", File: "a.go", Severity: SeverityMedium, Description: "Simplify 1"},
				},
			},
			{
				AgentName: "simplification",
				Issues: []Issue{
					{ID: "s-2", File: "b.go", Severity: SeverityMedium, Description: "Simplify 2"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		require.False(t, validatorCalled)
		require.Equal(t, input, validated)
	})

	t.Run("multiple simplification results pass through unchanged", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues:    []Issue{{ID: "q-1"}},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues:    []Issue{{ID: "q-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"}},
			},
			{
				AgentName: "simplification",
				Issues:    []Issue{{ID: "s-1", File: "b.go", Severity: SeverityMedium, Description: "Simplify 1"}},
			},
			{
				AgentName: "simplification",
				Issues:    []Issue{{ID: "s-2", File: "c.go", Severity: SeverityMedium, Description: "Simplify 2"}},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		// Both simplification results pass through
		require.Len(t, validated[1].Issues, 1)
		require.Equal(t, "s-1", validated[1].Issues[0].ID)
		require.Len(t, validated[2].Issues, 1)
		require.Equal(t, "s-2", validated[2].Issues[0].ID)
	})
}
