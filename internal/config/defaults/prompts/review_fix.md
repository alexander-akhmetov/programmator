# Review fix prompt
# This prompt is used when Claude needs to fix code review issues.
#
# Available variables:
#   {{.BaseBranch}} - the base branch for comparison
#   {{.Iteration}} - current review iteration number
#   {{.FilesList}} - formatted list of files to review
#   {{.IssuesMarkdown}} - formatted markdown of issues found

You are reviewing and fixing code issues found by automated code review.

## Context
- Base branch: {{.BaseBranch}}
- Review iteration: {{.Iteration}}

## Files to review
{{.FilesList}}

## Issues Found
The following issues were found by code review agents and need to be fixed:

{{.IssuesMarkdown}}

## Instructions
1. Review each issue carefully
2. Make the necessary fixes to address each issue
3. After fixing, commit your changes with a clear commit message
4. Report your status

## Important
- Fix ALL issues listed above
- Make clean, minimal fixes that address the specific issues
- Test your changes if possible
- Commit with message format: "fix: <brief description of fixes>"

## Session End Protocol
When you've completed your fixes, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - file1.go
    - file2.go
  summary: "Fixed N issues: brief description"
  commit_made: true
```

Status values:
- CONTINUE: Made fixes, ready for re-review
- DONE: All issues fixed, commit made
- BLOCKED: Cannot fix without human intervention (add error: field)

If blocked:
```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "What was attempted"
  error: "Description of what's blocking progress"
```
