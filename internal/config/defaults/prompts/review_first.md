# Comprehensive Review Prompt
# This prompt is used for the first (comprehensive) review phase.
# Runs all review agents for thorough code review with self-verification.
#
# Available variables:
#   {{.BaseBranch}} - the base branch for comparison
#   {{.Iteration}} - current review iteration number
#   {{.FilesList}} - formatted list of files to review
#   {{.IssuesMarkdown}} - formatted markdown of issues found by agents

Code review iteration {{.Iteration}}

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

## Step 2: Review Issues from Agents

The following issues were found by code review agents:

{{.IssuesMarkdown}}

## Step 2.5: Deduplicate Findings

Before verification:
- Same file:line + same issue from different agents - merge into one finding
- Cross-agent duplicates - merge, note both sources
- Remove exact duplicates

## Step 3: Self-Verification (CRITICAL)

For EACH issue reported above:
1. Read actual code at file:line
2. Check full context (20-30 lines around)
3. Verify issue is real, not a false positive
4. Check for existing mitigations

Classify as:
- CONFIRMED: Real issue, fix it
- FALSE POSITIVE: Doesn't exist or already mitigated - discard

IMPORTANT: Pre-existing issues (linter errors, failed tests) should also be fixed.
Do NOT reject issues just because they existed before this branch - fix them anyway.

## Step 4: Fix All Confirmed Issues

1. Fix all CONFIRMED issues (all types: bugs, tests, smells, docs, etc.)
2. Run tests and linter to verify fixes - ALL tests must pass, ALL linter issues resolved
{{- if .AutoCommit }}
3. Commit fixes: `git commit -m "fix: address code review findings"`
{{- end }}

## Step 5: Signal Completion

SIGNAL LOGIC - READ CAREFULLY:

DONE means "this iteration found ZERO issues" - NOT "I finished fixing issues".

Path A - NO confirmed issues found:
- You reviewed the code and found nothing to fix
- Output DONE status

Path B - Issues found AND fixed:
- You found issues and fixed them
- Output CONTINUE status
- The loop will run another review iteration to verify your fixes
- Your fixes might have introduced new issues - another iteration must check

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
  summary: "Fixed N issues: brief description"
{{- if .AutoCommit }}
  commit_made: true
{{- end }}
```

Status values:
- CONTINUE: Made fixes, ready for re-review
- DONE: Zero confirmed issues found - review passed
- BLOCKED: Cannot fix without human intervention (add error: field)

OUTPUT FORMAT: No markdown formatting (no **bold**, `code`, # headers). Plain text and - lists are fine.
