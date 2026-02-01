package review

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/worksonmyai/programmator/internal/codex"
)

// mockRunner implements codex.Runner for testing.
type mockRunner struct {
	runFunc func(ctx context.Context, name string, args ...string) (codex.Streams, func() error, error)
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) (codex.Streams, func() error, error) {
	return m.runFunc(ctx, name, args...)
}

func mockStreams(stdout string) codex.Streams {
	return codex.Streams{
		Stderr: strings.NewReader(""),
		Stdout: strings.NewReader(stdout),
	}
}

// newTestCodexAgent creates a CodexAgent with "echo" as command (exists in PATH)
// and a mock runner to control executor output.
func newTestCodexAgent(runner codex.Runner) *CodexAgent {
	agent := NewCodexAgent(CodexAgentConfig{
		Command: "echo",
		Prompt:  "Review code",
	})
	agent.executor.SetRunner(runner)
	return agent
}

func TestCodexAgent_Name(t *testing.T) {
	agent := NewCodexAgent(CodexAgentConfig{})
	require.Equal(t, "codex", agent.Name())
}

func TestCodexAgent_SkipsWhenBinaryMissing(t *testing.T) {
	// Use a command name that definitely doesn't exist
	agent := NewCodexAgent(CodexAgentConfig{
		Command: "nonexistent-codex-binary-xyz-123",
	})

	result, err := agent.Review(context.Background(), "/tmp", []string{"file.go"})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "codex", result.AgentName)
	require.Empty(t, result.Issues)
	require.Contains(t, result.Summary, "not available")
	require.Greater(t, result.Duration, time.Duration(0))
}

func TestCodexAgent_SkipsWhenCustomCommandMissing(t *testing.T) {
	agent := NewCodexAgent(CodexAgentConfig{
		Command: "codex-nonexistent-test-binary",
	})

	result, err := agent.Review(context.Background(), "/tmp", []string{"a.go", "b.go"})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Issues)
	require.Contains(t, result.Summary, "not available")
}

func TestCodexAgent_BuildPrompt(t *testing.T) {
	agent := NewCodexAgent(CodexAgentConfig{
		Prompt: "Review this code carefully.",
		Focus:  []string{"bugs", "security"},
	})

	prompt := agent.buildPrompt([]string{"main.go", "utils.go"})

	require.Contains(t, prompt, "Review this code carefully.")
	require.Contains(t, prompt, "bugs")
	require.Contains(t, prompt, "security")
	require.Contains(t, prompt, "main.go")
	require.Contains(t, prompt, "utils.go")
	require.Contains(t, prompt, "REVIEW_RESULT")
}

func TestCodexAgent_BuildPromptNoFocus(t *testing.T) {
	agent := NewCodexAgent(CodexAgentConfig{
		Prompt: "Review code.",
	})

	prompt := agent.buildPrompt([]string{"file.go"})

	require.Contains(t, prompt, "Review code.")
	require.NotContains(t, prompt, "Focus Areas")
	require.Contains(t, prompt, "file.go")
}

func TestCodexAgent_BuildPromptNoFiles(t *testing.T) {
	agent := NewCodexAgent(CodexAgentConfig{
		Prompt: "Review code.",
		Focus:  []string{"bugs"},
	})

	prompt := agent.buildPrompt(nil)

	require.Contains(t, prompt, "Review code.")
	require.Contains(t, prompt, "bugs")
	require.NotContains(t, prompt, "Files to Review")
}

func TestCodexAgent_ExecutorConfigMapping(t *testing.T) {
	// Verify all CodexAgentConfig fields are correctly mapped to the agent and executor.
	cfg := CodexAgentConfig{
		Command:         "test-codex",
		Model:           "gpt-5",
		ReasoningEffort: "high",
		TimeoutMs:       60000,
		Sandbox:         "read-only",
		Prompt:          "Test prompt",
		Focus:           []string{"bugs"},
	}

	agent := NewCodexAgent(cfg)

	require.Equal(t, "codex", agent.Name())
	require.Equal(t, "Test prompt", agent.prompt)
	require.Equal(t, []string{"bugs"}, agent.focus)
	require.NotNil(t, agent.executor)
	require.Equal(t, "test-codex", agent.executor.Command)
	require.Equal(t, "gpt-5", agent.executor.Model)
	require.Equal(t, "high", agent.executor.ReasoningEffort)
	require.Equal(t, 60000, agent.executor.TimeoutMs)
	require.Equal(t, "read-only", agent.executor.Sandbox)
}

func TestCodexAgent_ExecutorConfig(t *testing.T) {
	cfg := CodexAgentConfig{
		Command:         "my-codex",
		Model:           "gpt-5.2-codex",
		ReasoningEffort: "xhigh",
		TimeoutMs:       3600000,
		Sandbox:         "read-only",
		ProjectDoc:      "/path/to/doc.md",
		ErrorPatterns:   []string{"rate limit"},
	}

	agent := NewCodexAgent(cfg)

	require.Equal(t, "my-codex", agent.executor.Command)
	require.Equal(t, "gpt-5.2-codex", agent.executor.Model)
	require.Equal(t, "xhigh", agent.executor.ReasoningEffort)
	require.Equal(t, 3600000, agent.executor.TimeoutMs)
	require.Equal(t, "read-only", agent.executor.Sandbox)
	require.Equal(t, "/path/to/doc.md", agent.executor.ProjectDoc)
	require.Equal(t, []string{"rate limit"}, agent.executor.ErrorPatterns)
}

func TestCodexAgent_ReviewSuccess(t *testing.T) {
	stdout := `Some preamble text.
` + "```yaml" + `
REVIEW_RESULT:
  issues:
    - file: 'main.go'
      line: 10
      severity: 'high'
      category: 'bug'
      description: 'Nil pointer dereference'
      suggestion: 'Add nil check'
  summary: 'Found 1 issue'
` + "```"

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (codex.Streams, func() error, error) {
			return mockStreams(stdout), func() error { return nil }, nil
		},
	}
	agent := newTestCodexAgent(mock)

	result, err := agent.Review(context.Background(), "/tmp", []string{"main.go"})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "codex", result.AgentName)
	require.Len(t, result.Issues, 1)
	require.Equal(t, "main.go", result.Issues[0].File)
	require.Equal(t, 10, result.Issues[0].Line)
	require.Equal(t, SeverityHigh, result.Issues[0].Severity)
	require.Equal(t, "Nil pointer dereference", result.Issues[0].Description)
	require.Equal(t, "Found 1 issue", result.Summary)
	require.Greater(t, result.Duration, time.Duration(0))
}

func TestCodexAgent_ReviewExecutorError(t *testing.T) {
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (codex.Streams, func() error, error) {
			return codex.Streams{}, nil, errors.New("command not found")
		},
	}
	agent := newTestCodexAgent(mock)

	result, err := agent.Review(context.Background(), "/tmp", []string{"file.go"})

	require.NoError(t, err, "executor errors should be non-fatal")
	require.NotNil(t, result)
	require.Empty(t, result.Issues)
	require.Contains(t, result.Summary, "codex execution failed")
	require.Greater(t, result.Duration, time.Duration(0))
}

func TestCodexAgent_ReviewSetsWorkingDir(t *testing.T) {
	stdout := "```yaml\nREVIEW_RESULT:\n  issues: []\n  summary: 'No issues'\n```"
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (codex.Streams, func() error, error) {
			return mockStreams(stdout), func() error { return nil }, nil
		},
	}
	agent := newTestCodexAgent(mock)

	_, err := agent.Review(context.Background(), "/my/project", []string{"file.go"})
	require.NoError(t, err)
	// WorkingDir should NOT be set on the shared executor (race-safe copy is used instead)
	require.Empty(t, agent.executor.WorkingDir)
}

func TestCodexAgent_ReviewWaitError(t *testing.T) {
	stdout := "```yaml\nREVIEW_RESULT:\n  issues: []\n  summary: 'No issues'\n```"
	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (codex.Streams, func() error, error) {
			return mockStreams(stdout), func() error { return errors.New("exit status 1") }, nil
		},
	}
	agent := newTestCodexAgent(mock)

	result, err := agent.Review(context.Background(), "/tmp", []string{"file.go"})

	require.NoError(t, err, "wait errors should be non-fatal")
	require.NotNil(t, result)
	// The executor wraps wait errors but still returns output; review treats execution failure as non-fatal
	require.Contains(t, result.Summary, "codex execution failed")
	require.Greater(t, result.Duration, time.Duration(0))
}

func TestCodexAgent_ReviewContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (codex.Streams, func() error, error) {
			return codex.Streams{}, nil, context.Canceled
		},
	}
	agent := newTestCodexAgent(mock)

	result, err := agent.Review(ctx, "/tmp", []string{"file.go"})

	require.NoError(t, err, "context cancellation should be non-fatal")
	require.NotNil(t, result)
	require.Empty(t, result.Issues)
	require.Contains(t, result.Summary, "codex execution failed")
}

func TestCodexAgent_ReviewParseFailure(t *testing.T) {
	stdout := `Some output without valid YAML
REVIEW_RESULT:
  this is: [not valid yaml: {{{
`

	mock := &mockRunner{
		runFunc: func(_ context.Context, _ string, _ ...string) (codex.Streams, func() error, error) {
			return mockStreams(stdout), func() error { return nil }, nil
		},
	}
	agent := newTestCodexAgent(mock)

	result, err := agent.Review(context.Background(), "/tmp", []string{"file.go"})

	require.NoError(t, err, "parse errors should be non-fatal")
	require.NotNil(t, result)
	require.Empty(t, result.Issues)
	require.Contains(t, result.Summary, "failed to parse codex output as review YAML:")
	require.Greater(t, result.Duration, time.Duration(0))
}
