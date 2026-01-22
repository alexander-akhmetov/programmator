// Package prompt builds prompts for Claude Code invocations.
package prompt

import (
	"fmt"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/source"
)

const promptTemplate = `You are working on ticket %s: %s

## Current State
%s

## Progress Notes
%s

## Instructions
1. Read the ticket phases above (in the Design section)
2. Work on the FIRST uncompleted phase: [ ] (not [x])
3. Complete ONE phase per session - implement, test, verify
4. When done with the phase, output your status

## Current Phase
%s

## Session End Protocol
When you've completed your work for this iteration, you MUST end with exactly this block:

` + "```" + `
PROGRAMMATOR_STATUS:
  phase_completed: "%s"
  status: CONTINUE
  files_changed:
    - file1.py
    - file2.py
  summary: "One line describing what you did"
` + "```" + `

Status values:
- CONTINUE: Phase done or in progress, more work remains
- DONE: ALL phases complete, project finished
- BLOCKED: Cannot proceed without human intervention (add error: field)

If blocked:
` + "```" + `
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "What was attempted"
  error: "Description of what's blocking progress"
` + "```" + `
`

// Build creates a prompt from a work item and optional progress notes.
func Build(w *source.WorkItem, notes []string) string {
	currentPhase := w.CurrentPhase()
	phaseName := "null"
	currentPhaseStr := "All phases complete"
	if currentPhase != nil {
		phaseName = currentPhase.Name
		currentPhaseStr = fmt.Sprintf("**%s**", currentPhase.Name)
	}

	notesStr := "(No previous notes)"
	if len(notes) > 0 {
		noteLines := make([]string, 0, len(notes))
		for _, note := range notes {
			noteLines = append(noteLines, fmt.Sprintf("- %s", note))
		}
		notesStr = strings.Join(noteLines, "\n")
	}

	return fmt.Sprintf(
		promptTemplate,
		w.ID,
		w.Title,
		w.RawContent,
		notesStr,
		currentPhaseStr,
		phaseName,
	)
}

func BuildPhaseList(phases []source.Phase) string {
	lines := make([]string, 0, len(phases))
	for _, p := range phases {
		checkbox := "[ ]"
		if p.Completed {
			checkbox = "[x]"
		}
		lines = append(lines, fmt.Sprintf("- %s %s", checkbox, p.Name))
	}
	return strings.Join(lines, "\n")
}
