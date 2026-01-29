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
Work on the task described above. Complete the work and report your status when done.

## Session End Protocol
When you've completed your work for this iteration, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - file1.py
    - file2.py
  summary: "One line describing what you did"
```

Status values:
- CONTINUE: Making progress, more work remains
- DONE: Task complete
- BLOCKED: Cannot proceed without human intervention (add error: field)

If blocked:
```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "What was attempted"
  error: "Description of what's blocking progress"
```
