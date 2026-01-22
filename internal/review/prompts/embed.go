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
