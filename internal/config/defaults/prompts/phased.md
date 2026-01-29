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

## Instructions
1. Read the ticket phases above (in the Design section)
2. Work on the FIRST uncompleted phase: [ ] (not [x])
3. Complete ONE phase per session - implement, test, verify
4. When done with the phase, output your status

## Current Phase
**{{.CurrentPhase}}**

## Session End Protocol
When you've completed your work for this iteration, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: "{{.CurrentPhaseName}}"
  status: CONTINUE
  files_changed:
    - file1.py
    - file2.py
  summary: "One line describing what you did"
```

Status values:
- CONTINUE: Phase done or in progress, more work remains
- DONE: ALL phases complete, project finished
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
