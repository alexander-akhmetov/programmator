package review

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/codex"
)

// CodexAgent implements Agent using the Codex CLI.
// If the codex binary is not available, Review returns an empty result (no issues).
type CodexAgent struct {
	executor *codex.Executor
	prompt   string
	focus    []string
}

// CodexAgentConfig holds configuration for creating a CodexAgent.
type CodexAgentConfig struct {
	Command         string
	Model           string
	ReasoningEffort string
	TimeoutMs       int
	Sandbox         string
	ProjectDoc      string
	ErrorPatterns   []string
	Prompt          string
	Focus           []string
	OutputHandler   func(string)
}

// NewCodexAgent creates a new CodexAgent with the given config.
func NewCodexAgent(cfg CodexAgentConfig) *CodexAgent {
	executor := &codex.Executor{
		Command:         cfg.Command,
		Model:           cfg.Model,
		ReasoningEffort: cfg.ReasoningEffort,
		TimeoutMs:       cfg.TimeoutMs,
		Sandbox:         cfg.Sandbox,
		ProjectDoc:      cfg.ProjectDoc,
		ErrorPatterns:   cfg.ErrorPatterns,
		OutputHandler:   cfg.OutputHandler,
	}

	return &CodexAgent{
		executor: executor,
		prompt:   cfg.Prompt,
		focus:    cfg.Focus,
	}
}

// Name returns "codex".
func (a *CodexAgent) Name() string {
	return "codex"
}

// Review runs a Codex review. If codex is unavailable, returns empty result.
func (a *CodexAgent) Review(ctx context.Context, workingDir string, filesChanged []string) (*Result, error) {
	start := time.Now()
	result := &Result{
		AgentName: "codex",
		Issues:    make([]Issue, 0),
	}

	cmd := a.executor.Command
	if cmd == "" {
		cmd = "codex"
	}
	if _, found := codex.DetectBinary(cmd); !found {
		result.Summary = "codex binary not available, skipping"
		result.Duration = time.Since(start)
		return result, nil
	}

	// Copy the executor to avoid race conditions when Review() is called
	// concurrently (e.g., parallel agent execution via getOrCreateAgent cache).
	exec := *a.executor
	exec.WorkingDir = workingDir

	prompt := a.buildPrompt(filesChanged)
	res := exec.Run(ctx, prompt)
	if res.Error != nil {
		result.Summary = fmt.Sprintf("codex execution failed: %v", res.Error)
		result.Duration = time.Since(start)
		return result, nil //nolint:nilerr // execution failure is non-fatal; return empty result
	}

	issues, summary, parseErr := parseReviewOutput(res.Output)
	if parseErr != nil {
		result.Summary = fmt.Sprintf("failed to parse codex output as review YAML: %v", parseErr)
		result.Duration = time.Since(start)
		return result, nil //nolint:nilerr // parse failure is non-fatal; return empty result
	}

	result.Issues = issues
	result.Summary = summary
	result.Duration = time.Since(start)
	return result, nil
}

// buildPrompt constructs the review prompt for Codex.
func (a *CodexAgent) buildPrompt(filesChanged []string) string {
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

Respond with a YAML block containing your findings.

IMPORTANT: Always single-quote all string values. Do NOT use double-quoted strings — they cause parse errors with backslashes like \d, \w, \s. For multiline values, use ` + "`|`" + ` block scalars. If a value contains a single quote, escape it by doubling: ` + "`''`" + `.

` + "```yaml" + `
REVIEW_RESULT:
  issues:
    - file: 'path/to/file.go'
      line: 42
      severity: 'high'  # critical, high, medium, low, info
      category: 'error handling'
      description: 'Error is ignored without logging'
      suggestion: 'Add error logging or return the error'
  summary: 'Brief summary of findings'
` + "```" + `

If no issues found:
` + "```yaml" + `
REVIEW_RESULT:
  issues: []
  summary: 'No issues found'
` + "```")

	return b.String()
}
