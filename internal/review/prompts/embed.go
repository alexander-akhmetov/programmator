// Package prompts provides embedded default prompts for review agents.
package prompts

import (
	_ "embed"
)

//go:embed quality.md
var QualityPrompt string

//go:embed security.md
var SecurityPrompt string

//go:embed linter.md
var LinterPrompt string

//go:embed implementation.md
var ImplementationPrompt string

//go:embed testing.md
var TestingPrompt string

//go:embed simplification.md
var SimplificationPrompt string

//go:embed claudemd.md
var ClaudeMDPrompt string

//go:embed simplification_validator.md
var SimplificationValidatorPrompt string
