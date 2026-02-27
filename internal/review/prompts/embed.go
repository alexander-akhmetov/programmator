// Package prompts provides embedded default prompts for review agents.
package prompts

import (
	_ "embed"
)

//go:embed bug_shallow.md
var BugShallowPrompt string

//go:embed bug_deep.md
var BugDeepPrompt string

//go:embed architect.md
var ArchitectPrompt string

//go:embed silent_failures.md
var SilentFailuresPrompt string

//go:embed type_design.md
var TypeDesignPrompt string

//go:embed comments.md
var CommentsPrompt string

//go:embed simplification.md
var SimplificationPrompt string

//go:embed claudemd.md
var ClaudeMDPrompt string

//go:embed linter.md
var LinterPrompt string

//go:embed simplification_validator.md
var SimplificationValidatorPrompt string

//go:embed issue_validator.md
var IssueValidatorPrompt string
