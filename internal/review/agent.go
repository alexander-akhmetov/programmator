package review

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/worksonmyai/programmator/internal/llm"
)

// Result holds the result of a single agent review.
type Result struct {
	AgentName  string
	Issues     []Issue
	Summary    string
	Error      error
	Duration   time.Duration
	TokensUsed int
}

// Issue represents a single review issue found by an agent.
type Issue struct {
	File        string   `yaml:"file"`
	Line        int      `yaml:"line,omitempty"`
	Severity    Severity `yaml:"severity"`
	Category    string   `yaml:"category"`
	Description string   `yaml:"description"`
	Suggestion  string   `yaml:"suggestion,omitempty"`
}

// Severity represents the severity level of an issue.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// Agent defines the interface for code review agents.
type Agent interface {
	// Name returns the agent's name.
	Name() string

	// Review runs the review and returns the result.
	// The context should be used for cancellation and timeouts.
	// workingDir is the directory containing the code to review.
	// filesChanged is the list of files that have been modified.
	Review(ctx context.Context, workingDir string, filesChanged []string) (*Result, error)
}

// ClaudeAgent implements ReviewAgent using Claude Code.
type ClaudeAgent struct {
	name         string
	focus        []string
	prompt       string
	timeout      time.Duration
	claudeArgs   []string
	settingsJSON string
	invoker      llm.Invoker
}

// ClaudeAgentOption is a functional option for ClaudeAgent.
type ClaudeAgentOption func(*ClaudeAgent)

// WithTimeout sets the timeout for Claude invocations.
func WithTimeout(d time.Duration) ClaudeAgentOption {
	return func(a *ClaudeAgent) {
		a.timeout = d
	}
}

// WithClaudeArgs sets additional Claude CLI arguments.
func WithClaudeArgs(args []string) ClaudeAgentOption {
	return func(a *ClaudeAgent) {
		a.claudeArgs = args
	}
}

// WithSettingsJSON sets the --settings JSON for guard mode / permission hooks.
func WithSettingsJSON(settingsJSON string) ClaudeAgentOption {
	return func(a *ClaudeAgent) {
		a.settingsJSON = settingsJSON
	}
}

// NewClaudeAgent creates a new ClaudeAgent.
func NewClaudeAgent(name string, focus []string, prompt string, opts ...ClaudeAgentOption) *ClaudeAgent {
	agent := &ClaudeAgent{
		name:    name,
		focus:   focus,
		prompt:  prompt,
		timeout: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(agent)
	}

	return agent
}

// Name returns the agent's name.
func (a *ClaudeAgent) Name() string {
	return a.name
}

// Review runs the code review using Claude.
func (a *ClaudeAgent) Review(ctx context.Context, workingDir string, filesChanged []string) (*Result, error) {
	start := time.Now()
	result := &Result{
		AgentName: a.name,
		Issues:    make([]Issue, 0),
	}

	prompt := a.buildPrompt(filesChanged)

	output, err := a.invokeClaude(ctx, workingDir, prompt)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result, err
	}

	issues, summary, err := parseReviewOutput(output)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse review output: %w", err)
		result.Duration = time.Since(start)
		return result, result.Error
	}

	result.Issues = issues
	result.Summary = summary
	result.Duration = time.Since(start)

	return result, nil
}

// buildPrompt constructs the review prompt for Claude.
func (a *ClaudeAgent) buildPrompt(filesChanged []string) string {
	var b strings.Builder

	b.WriteString(a.prompt)
	b.WriteString("\n\n")

	if len(a.focus) > 0 {
		b.WriteString("## Focus Areas\n")
		for _, f := range a.focus {
			b.WriteString("- ")
			b.WriteString(f)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(filesChanged) > 0 {
		b.WriteString("## Files to Review\n")
		for _, f := range filesChanged {
			b.WriteString("- ")
			b.WriteString(f)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(`## Output Format

Respond with a YAML block containing your findings:

` + "```yaml" + `
REVIEW_RESULT:
  issues:
    - file: "path/to/file.go"
      line: 42
      severity: high  # critical, high, medium, low, info
      category: "error handling"
      description: "Error is ignored without logging"
      suggestion: "Add error logging or return the error"
  summary: "Brief summary of findings"
` + "```" + `

If no issues found:
` + "```yaml" + `
REVIEW_RESULT:
  issues: []
  summary: "No issues found"
` + "```")

	return b.String()
}

// invokeClaude runs Claude with the given prompt via llm.Invoker.
func (a *ClaudeAgent) invokeClaude(ctx context.Context, workingDir, promptText string) (string, error) {
	inv := a.invoker
	if inv == nil {
		inv = llm.NewClaudeInvoker(llm.EnvConfig{})
	}

	extraFlags := strings.Join(a.claudeArgs, " ")

	opts := llm.InvokeOptions{
		WorkingDir:   workingDir,
		ExtraFlags:   extraFlags,
		SettingsJSON: a.settingsJSON,
		Timeout:      int(a.timeout.Seconds()),
	}

	res, err := inv.Invoke(ctx, promptText, opts)
	if err != nil {
		return "", fmt.Errorf("claude invocation failed: %w", err)
	}
	return res.Text, nil
}

// MockAgent is a mock implementation for testing.
type MockAgent struct {
	name       string
	reviewFunc func(ctx context.Context, workingDir string, filesChanged []string) (*Result, error)
}

// NewMockAgent creates a new MockAgent.
func NewMockAgent(name string) *MockAgent {
	return &MockAgent{
		name: name,
		reviewFunc: func(_ context.Context, _ string, _ []string) (*Result, error) {
			return &Result{
				AgentName: name,
				Issues:    []Issue{},
				Summary:   "Mock review passed",
			}, nil
		},
	}
}

// Name returns the mock agent's name.
func (m *MockAgent) Name() string {
	return m.name
}

// Review runs the mock review function.
func (m *MockAgent) Review(ctx context.Context, workingDir string, filesChanged []string) (*Result, error) {
	return m.reviewFunc(ctx, workingDir, filesChanged)
}

// SetReviewFunc sets the mock review function.
func (m *MockAgent) SetReviewFunc(f func(ctx context.Context, workingDir string, filesChanged []string) (*Result, error)) {
	m.reviewFunc = f
}
