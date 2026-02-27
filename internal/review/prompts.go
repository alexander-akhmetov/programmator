package review

import (
	"github.com/alexander-akhmetov/programmator/internal/review/prompts"
)

// GetDefaultPrompt returns the default prompt for an agent by name.
// Falls back to a generic prompt if no specific prompt is available.
func GetDefaultPrompt(agentName string) string {
	switch agentName {
	case "error-handling", "logic":
		return prompts.QualityPrompt
	case "security":
		return prompts.SecurityPrompt
	case "linter":
		return prompts.LinterPrompt
	case "implementation":
		return prompts.ImplementationPrompt
	case "testing":
		return prompts.TestingPrompt
	case "simplification":
		return prompts.SimplificationPrompt
	case "claudemd":
		return prompts.ClaudeMDPrompt
	case "simplification-validator":
		return prompts.SimplificationValidatorPrompt
	case "issue-validator":
		return prompts.IssueValidatorPrompt
	default:
		return defaultGenericPrompt
	}
}

// GetDefaultPromptForAgent returns the default prompt for an agent config.
// Currently delegates to GetDefaultPrompt; the Executor field is reserved
// for future executor backends.
func GetDefaultPromptForAgent(cfg AgentConfig) string {
	return GetDefaultPrompt(cfg.Name)
}

const defaultGenericPrompt = `# Code Review

You are a code review agent. Review the specified files for issues.

## Review Guidelines

- Identify bugs, potential issues, and areas for improvement
- Prioritize issues by severity (critical, high, medium, low, info)
- Provide specific suggestions for how to fix each issue
- Focus on actionable feedback that improves code quality
`
