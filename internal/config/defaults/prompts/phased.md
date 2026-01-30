# Phased execution prompt
# This prompt is used when the work item has phases (checkboxed tasks).
#
# Available variables:
#   {{.ID}} - work item identifier (ticket ID or plan filename)
#   {{.Title}} - human-readable title
#   {{.RawContent}} - full content of the work item
#   {{.Notes}} - formatted progress notes from previous iterations
#   {{.CurrentPhase}} - name of the current incomplete phase (or "All phases complete")
#   {{.CurrentPhaseName}} - raw phase name for status block (or "null")

You are working on ticket {{.ID}}: {{.Title}}

## Current State
{{.RawContent}}

## Progress Notes
{{.Notes}}

## Current Phase
**{{.CurrentPhase}}**

## Instructions

STEP 0 - ANNOUNCE:
Before starting work, output a brief overview (up to 200 words):
- Which phase you picked and its title
- What the phase will accomplish
- Key files or components involved

STEP 1 - IMPLEMENT:
- Read the ticket/plan to understand the full context
- Implement ALL items in the current phase
- Write tests for the implementation

STEP 2 - VALIDATE:
- Run ALL validation commands from the plan (test suites, linters, etc.)
- Fix any failures, repeat until ALL pass
- ALL tests must pass and ALL linter issues must be resolved before proceeding

STEP 3 - COMPLETE:
- Check if more uncompleted phases remain

CRITICAL: Complete ONE phase per iteration, then STOP.
Do NOT continue to the next phase - the external loop will call you again.

## Session End Protocol
When you've completed your work for this iteration, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: "{{.CurrentPhaseName}}"
  status: CONTINUE
  files_changed:
    - file1.go
    - file2.go
  summary: "One line describing what you did"
```

Status values:
- CONTINUE: Phase done or in progress, more work remains
- DONE: ALL phases complete, project finished
- BLOCKED: Cannot proceed after reasonable fix attempts (add error: field)

If blocked:
```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "What was attempted"
  error: "Description of what's blocking progress"
```
