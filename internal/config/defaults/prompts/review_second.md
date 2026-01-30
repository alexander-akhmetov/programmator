# Critical Issues Review Prompt
# This prompt is used for the final review phase focusing on critical/major issues only.
# Uses fewer agents and filters for high-severity issues.
#
# Available variables:
#   {{.BaseBranch}} - the base branch for comparison
#   {{.Iteration}} - current review iteration number
#   {{.FilesList}} - formatted list of files to review
#   {{.IssuesMarkdown}} - formatted markdown of issues found by agents

Second code review pass - iteration {{.Iteration}}

Focus only on critical and major issues. Ignore style/minor issues.

## Step 1: Get Branch Context

{{- if .BaseBranch }}
Run both commands to understand what was done:
- `git log {{.BaseBranch}}..HEAD --oneline` - see commit history
- `git diff {{.BaseBranch}}...HEAD` - see actual code changes
{{- else }}
No base branch provided. Use these commands to understand recent changes:
- `git log -n 20 --oneline`
- `git diff`
- `git diff --cached`
{{- end }}

## Step 2: Review Critical/Major Issues

The following critical/major issues were found by code review agents:

{{.IssuesMarkdown}}

## Step 3: Verify Each Finding

For each issue reported:
1. Read actual code at file:line
2. Verify issue is real (not false positive)
3. Check if it's truly critical/major severity

## Step 4: Act on Verified Findings

IMPORTANT: Pre-existing issues (linter errors, failed tests) should also be fixed.
Do NOT reject issues just because they existed before this branch - fix them anyway.

SIGNAL LOGIC - READ CAREFULLY:

DONE means "this iteration found ZERO issues" - NOT "I finished fixing issues".

Path A - NO issues found in this iteration:
- You reviewed the code and found nothing critical/major to fix
- Output DONE status

Path B - Issues found AND fixed:
1. Fix verified critical/major issues only
2. Run tests and linter - ALL tests must pass, ALL linter issues resolved
3. Commit fixes: `git commit -m "fix: address code review findings"`
4. Output CONTINUE status
   The loop will run another review iteration to verify your fixes.
   Your fixes might have introduced new issues - another iteration must check.

Path C - Issues found but cannot fix:
- Output BLOCKED status with error description

## Session End Protocol

When you've completed your review, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - file1.go
    - file2.go
  summary: "Fixed N critical issues: brief description"
  commit_made: true
```

Status values:
- CONTINUE: Made fixes, ready for re-review
- DONE: Zero critical/major issues found - review passed
- BLOCKED: Cannot fix without human intervention (add error: field)

OUTPUT FORMAT: No markdown formatting (no **bold**, `code`, # headers). Plain text and - lists are fine.
