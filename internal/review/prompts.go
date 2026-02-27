package review

import (
	"github.com/alexander-akhmetov/programmator/internal/review/prompts"
)

// GetDefaultPrompt returns the default prompt for an agent by name.
// Falls back to a generic prompt if no specific prompt is available.
func GetDefaultPrompt(agentName string) string {
	switch agentName {
	case "bug-shallow":
		return prompts.BugShallowPrompt
	case "bug-deep":
		return prompts.BugDeepPrompt
	case "architect":
		return prompts.ArchitectPrompt
	case "simplification":
		return prompts.SimplificationPrompt
	case "silent-failures":
		return prompts.SilentFailuresPrompt
	case "claudemd":
		return prompts.ClaudeMDPrompt
	case "type-design":
		return prompts.TypeDesignPrompt
	case "comments":
		return prompts.CommentsPrompt
	case "linter":
		return prompts.LinterPrompt
	case "simplification-validator":
		return prompts.SimplificationValidatorPrompt
	case "issue-validator":
		return prompts.IssueValidatorPrompt
	default:
		return defaultGenericPrompt
	}
}

// GetDefaultPromptForAgent returns the default prompt for an agent config.
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
