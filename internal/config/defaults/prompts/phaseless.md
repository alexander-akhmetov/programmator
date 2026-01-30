# Phaseless execution prompt
# This prompt is used when the work item has no phases (single task).
#
# Available variables:
#   {{.ID}} - work item identifier (ticket ID or plan filename)
#   {{.Title}} - human-readable title
#   {{.RawContent}} - full content of the work item
#   {{.Notes}} - formatted progress notes from previous iterations

You are working on ticket {{.ID}}: {{.Title}}

## Current State
{{.RawContent}}

## Progress Notes
{{.Notes}}

## Instructions

STEP 0 - ANNOUNCE:
Before starting work, output a brief overview (up to 200 words):
- What the task will accomplish
- Key files or components involved

STEP 1 - IMPLEMENT:
- Read the task description to understand the full context
- Implement the requested changes
- Write tests for the implementation

STEP 2 - VALIDATE:
- Run ALL validation commands (test suites, linters, etc.)
- Fix any failures, repeat until ALL pass
- ALL tests must pass and ALL linter issues must be resolved

STEP 3 - COMPLETE:
- Report your status

## Session End Protocol
When you've completed your work for this iteration, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - file1.go
    - file2.go
  summary: "One line describing what you did"
```

Status values:
- CONTINUE: Making progress, more work remains
- DONE: Task complete
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
