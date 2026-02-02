package review

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/worksonmyai/programmator/internal/llm"
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
			WithEnvConfig(llm.EnvConfig{
				ClaudeConfigDir: "/custom/config",
				AnthropicAPIKey: "test-key",
			}),
		)

		require.Equal(t, 10*time.Minute, agent.timeout)
		require.Equal(t, []string{"--verbose"}, agent.claudeArgs)
		require.Equal(t, `{"hooks":{}}`, agent.settingsJSON)
		require.Equal(t, "/custom/config", agent.envConfig.ClaudeConfigDir)
		require.Equal(t, "test-key", agent.envConfig.AnthropicAPIKey)
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

func TestDefaultAgentFactory_PassesEnvConfig(t *testing.T) {
	cfg := Config{
		MaxIterations: 3,
		Timeout:       120,
		EnvConfig: llm.EnvConfig{
			ClaudeConfigDir: "/custom/claude/config",
			AnthropicAPIKey: "sk-test-key",
		},
	}

	runner := NewRunner(cfg, nil)
	agent := runner.defaultAgentFactory(AgentConfig{Name: "test", Focus: []string{"bugs"}}, "default prompt")

	claudeAgent, ok := agent.(*ClaudeAgent)
	require.True(t, ok)
	require.Equal(t, "/custom/claude/config", claudeAgent.envConfig.ClaudeConfigDir)
	require.Equal(t, "sk-test-key", claudeAgent.envConfig.AnthropicAPIKey)
}

func TestDefaultAgentFactory_EmptyEnvConfig(t *testing.T) {
	cfg := Config{
		MaxIterations: 3,
	}

	runner := NewRunner(cfg, nil)
	agent := runner.defaultAgentFactory(AgentConfig{Name: "test", Focus: []string{"bugs"}}, "default prompt")

	claudeAgent, ok := agent.(*ClaudeAgent)
	require.True(t, ok)
	require.Equal(t, llm.EnvConfig{}, claudeAgent.envConfig)
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

func TestRunner_RunIteration(t *testing.T) {
	t.Run("runs agents in parallel", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      true,
			Agents: []AgentConfig{
				{Name: "agent1"},
				{Name: "agent2"},
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

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{"file.go"})
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, 0, result.TotalIssues)
		require.Len(t, result.Results, 2)
	})

	t.Run("runs agents sequentially when not parallel", func(t *testing.T) {
		callOrder := []string{}
		cfg := Config{
			MaxIterations: 3,
			Parallel:      false,
			Agents: []AgentConfig{
				{Name: "first"},
				{Name: "second"},
			},
		}

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

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{})
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, []string{"first", "second"}, callOrder)
	})

	t.Run("counts issues correctly", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      true,
			Agents: []AgentConfig{
				{Name: "agent1"},
				{Name: "agent2"},
			},
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

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{})
		require.NoError(t, err)
		require.False(t, result.Passed)
		require.Equal(t, 2, result.TotalIssues)
	})

	t.Run("agent errors fail the iteration", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      true,
			Agents: []AgentConfig{
				{Name: "agent1"},
				{Name: "agent2"},
			},
		}

		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			if agentCfg.Name == "agent1" {
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return nil, fmt.Errorf("agent failed")
				})
			} else {
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: agentCfg.Name,
						Issues:    []Issue{},
					}, nil
				})
			}
			return mock
		})

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{})
		require.NoError(t, err)
		require.False(t, result.Passed)
		require.Equal(t, 1, result.TotalIssues)
	})

	t.Run("assigns IDs to issues", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      true,
			Agents:        []AgentConfig{{Name: "agent1"}},
		}
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

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{"a.go"})
		require.NoError(t, err)
		require.Len(t, result.Results, 1)
		require.NotEmpty(t, result.Results[0].Issues[0].ID)
	})

	t.Run("empty agents list passes", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Agents:        []AgentConfig{},
		}
		runner := NewRunner(cfg, nil)

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{})
		require.NoError(t, err)
		require.True(t, result.Passed)
		require.Equal(t, 0, result.TotalIssues)
		require.Empty(t, result.Results)
	})

	t.Run("always validates issues", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      false,
			Agents:        []AgentConfig{{Name: "quality"}},
		}
		runner := NewRunner(cfg, nil)

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
							{ID: "id-keep", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Real bug"},
							{ID: "id-drop", Verdict: "false_positive", File: "b.go", Severity: SeverityLow, Category: "style", Description: "False positive"},
						},
						Summary: "Validated 1 of 2",
					}, nil
				})
			}
			return mock
		})

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{"a.go"})
		require.NoError(t, err)
		require.Equal(t, 1, result.TotalIssues)
		require.Len(t, result.Results[0].Issues, 1)
		require.Equal(t, "id-keep", result.Results[0].Issues[0].ID)
	})

	t.Run("filters using generated IDs and verdicts", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      false,
			Agents:        []AgentConfig{{Name: "quality"}},
		}
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
						var issues []Issue
						for _, iss := range parsed.Issues {
							if iss.File == "a.go" {
								issues = append(issues, Issue{ID: iss.ID, Verdict: "valid"})
							} else {
								issues = append(issues, Issue{ID: iss.ID, Verdict: "false_positive"})
							}
						}
						return &Result{
							AgentName: "issue-validator",
							Issues:    issues,
							Summary:   "Validated 1 of 2",
						}, nil
					}
					return &Result{AgentName: "issue-validator", Issues: []Issue{}}, nil
				})
			}
			return mock
		})

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{"a.go", "b.go"})
		require.NoError(t, err)
		require.Equal(t, 1, result.TotalIssues)
		require.Len(t, result.Results[0].Issues, 1)
		require.Equal(t, "Real bug", result.Results[0].Issues[0].Description)
		require.NotEmpty(t, result.Results[0].Issues[0].ID)
	})

	t.Run("both simplification and issue validation run", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      false,
			Agents: []AgentConfig{
				{Name: "quality"},
				{Name: "simplification"},
			},
		}
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
						var issues []Issue
						for _, iss := range parsed.Issues {
							if iss.File == "a.go" {
								issues = append(issues, Issue{ID: iss.ID, Verdict: "valid"})
							} else {
								issues = append(issues, Issue{ID: iss.ID, Verdict: "false_positive"})
							}
						}
						return &Result{AgentName: "issue-validator", Issues: issues}, nil
					}
					return &Result{AgentName: "issue-validator", Issues: []Issue{}}, nil
				})
			}
			return mock
		})

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{"a.go", "b.go", "c.go", "d.go"})
		require.NoError(t, err)
		require.True(t, simpValidatorCalled)
		require.True(t, issueValidatorCalled)

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

		require.NotNil(t, simpResult)
		require.Len(t, simpResult.Issues, 1)
		require.Equal(t, "Good simplification", simpResult.Issues[0].Description)

		require.Equal(t, 2, result.TotalIssues)
	})
}

func TestRunner_RunIteration_ValidatorsAlwaysRun(t *testing.T) {
	t.Run("validators run on every iteration call", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      false,
			Agents:        []AgentConfig{{Name: "quality"}},
		}
		runner := NewRunner(cfg, nil)

		iterationCount := 0
		issueValidatorCallCount := 0

		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			switch agentCfg.Name {
			case "quality":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					iterationCount++
					return &Result{
						AgentName: "quality",
						Issues: []Issue{
							{File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: fmt.Sprintf("Bug iter %d", iterationCount)},
						},
					}, nil
				})
			case "issue-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					issueValidatorCallCount++
					return &Result{
						AgentName: "issue-validator",
						Issues:    []Issue{},
						Summary:   "All false positives",
					}, nil
				})
			}
			return mock
		})

		// Call RunIteration multiple times to verify validators run each time
		for range 3 {
			_, err := runner.RunIteration(context.Background(), "/tmp", []string{"a.go"})
			require.NoError(t, err)
		}

		require.Equal(t, 3, iterationCount)
		require.Equal(t, 3, issueValidatorCallCount, "issue validator should run on every iteration")
	})

	t.Run("validator failure falls back to raw agent results in RunIteration", func(t *testing.T) {
		cfg := Config{
			MaxIterations: 3,
			Parallel:      false,
			Agents:        []AgentConfig{{Name: "quality"}},
		}
		runner := NewRunner(cfg, nil)

		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			switch agentCfg.Name {
			case "quality":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return &Result{
						AgentName: "quality",
						Issues: []Issue{
							{File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Real bug"},
						},
					}, nil
				})
			case "issue-validator":
				mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
					return nil, fmt.Errorf("validator crashed")
				})
			}
			return mock
		})

		result, err := runner.RunIteration(context.Background(), "/tmp", []string{"a.go"})
		require.NoError(t, err)
		// Should keep original issues on validator failure (fallback behavior)
		require.Equal(t, 1, result.TotalIssues)
		require.False(t, result.Passed)
	})
}

func TestRunner_DefaultFactory_CodexAgent(t *testing.T) {
	cfg := Config{
		MaxIterations: 3,
		Codex: CodexSettings{
			Command:         "test-codex",
			Model:           "gpt-5.2-codex",
			ReasoningEffort: "xhigh",
			TimeoutMs:       3600000,
			Sandbox:         "read-only",
		},
	}

	runner := NewRunner(cfg, nil)
	agent := runner.defaultAgentFactory(AgentConfig{Name: "codex", Focus: []string{"bugs"}}, "default prompt")

	codexAgent, ok := agent.(*CodexAgent)
	require.True(t, ok, "codex agent config should produce a CodexAgent")
	require.Equal(t, "codex", codexAgent.Name())
	require.Equal(t, "test-codex", codexAgent.executor.Command)
	require.Equal(t, "gpt-5.2-codex", codexAgent.executor.Model)
}

func TestRunner_ValidateIssues(t *testing.T) {
	t.Run("filters issues by validator verdict", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "confirmed-1", Verdict: "valid", File: "x.go", Severity: SeverityHigh, Category: "bugs", Description: "Confirmed"},
						{ID: "filtered-out", Verdict: "false_positive", File: "y.go", Severity: SeverityLow, Category: "style", Description: "FP"},
					},
					Summary: "1 confirmed, 1 false positive",
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
							{ID: "quality-1", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
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

	t.Run("orphan IDs from validator are ignored and missing IDs kept", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "confirmed-1", Verdict: "valid", File: "x.go", Severity: SeverityHigh, Description: "Real"},
						{ID: "orphan-id", Verdict: "valid", File: "z.go", Severity: SeverityHigh, Description: "Orphan"},
						{ID: "will-drop", Verdict: "false_positive", File: "y.go", Severity: SeverityLow, Description: "Drop"},
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
						{ID: "id-1", Verdict: "valid", File: "x.go", Severity: SeverityMedium, Description: "Modified description"},
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

	t.Run("validator returning empty issues keeps all originals", func(t *testing.T) {
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
		// Empty validator output = no verdicts = keep all (safe default)
		require.Len(t, validated[0].Issues, 2)
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

	t.Run("issues without IDs in input are kept as safe default", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Bug"},
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
		// Both kept: id-1 has verdict "valid", empty ID has no verdict match → kept
		require.Len(t, validated[0].Issues, 2)
	})

	t.Run("validator with mixed IDs uses verdicts for filtering", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Description: "Confirmed with ID"},
						{ID: "id-2", Verdict: "false_positive", File: "c.go", Severity: SeverityMedium, Description: "FP"},
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
		// id-1 verdict=valid → kept; id-2 verdict=false_positive → dropped
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
						{ID: "q-1", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Description: "Quality confirmed"},
						{ID: "q-2", Verdict: "false_positive", File: "c.go", Severity: SeverityLow, Description: "Quality FP"},
						{ID: "s-1", Verdict: "valid", File: "b.go", Severity: SeverityCritical, Description: "Security confirmed"},
						{ID: "s-2", Verdict: "false_positive", File: "d.go", Severity: SeverityMedium, Description: "Security FP"},
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
					Issues:    []Issue{{ID: "q-1", Verdict: "valid"}},
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

	t.Run("issues without verdict from validator are kept", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Description: "Confirmed"},
						{ID: "id-2", Verdict: "false_positive", File: "b.go", Severity: SeverityLow, Description: "FP"},
						{ID: "id-3", File: "c.go", Severity: SeverityMedium, Description: "No verdict"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Confirmed"},
					{ID: "id-2", File: "b.go", Severity: SeverityLow, Category: "style", Description: "FP"},
					{ID: "id-3", File: "c.go", Severity: SeverityMedium, Category: "logic", Description: "No verdict"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		// id-1 (valid) → kept, id-2 (false_positive) → dropped, id-3 (no verdict) → kept
		require.Len(t, validated[0].Issues, 2)
		require.Equal(t, "id-1", validated[0].Issues[0].ID)
		require.Equal(t, "id-3", validated[0].Issues[1].ID)
	})

	t.Run("issues not in validator output are kept as safe default", func(t *testing.T) {
		cfg := Config{MaxIterations: 3}
		runner := NewRunner(cfg, nil)
		runner.SetAgentFactory(func(agentCfg AgentConfig, _ string) Agent {
			mock := NewMockAgent(agentCfg.Name)
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*Result, error) {
				return &Result{
					AgentName: agentCfg.Name,
					Issues: []Issue{
						{ID: "id-1", Verdict: "valid", File: "a.go", Severity: SeverityHigh, Description: "Confirmed"},
					},
				}, nil
			})
			return mock
		})

		input := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id-1", File: "a.go", Severity: SeverityHigh, Category: "bugs", Description: "Confirmed"},
					{ID: "id-2", File: "b.go", Severity: SeverityMedium, Category: "style", Description: "Not in validator output"},
				},
			},
		}

		validated, err := runner.ValidateIssues(context.Background(), "/tmp", input)
		require.NoError(t, err)
		// id-1 (valid) → kept, id-2 (not in output) → kept (safe default)
		require.Len(t, validated[0].Issues, 2)
	})
}
