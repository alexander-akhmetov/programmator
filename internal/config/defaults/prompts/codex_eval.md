# Codex Evaluation Prompt
# Evaluates Codex review findings and applies fixes.
#
# Available variables:
#   {{.CodexOutput}} - the output from Codex review
#   {{.BaseBranch}} - the base branch for comparison (optional)
#   {{.FilesList}} - formatted list of changed files (optional)
#   {{.WorkItemID}} - ticket or plan ID
#   {{.Title}} - ticket or plan title
#   {{.RawContent}} - full ticket/plan content
#   {{.AutoCommit}} - whether to auto-commit fixes

## Context

You are evaluating findings from a Codex code review for work item {{.WorkItemID}}: {{.Title}}

{{- if .BaseBranch }}

Base branch: `{{.BaseBranch}}`
{{- end }}

{{- if .FilesList }}

Changed files:
{{.FilesList}}
{{- end }}

## Codex Review Output

```
{{.CodexOutput}}
```

## Instructions

1. **Evaluate each finding**: Determine if the issue is real and actionable, or a false positive.
2. **Fix confirmed issues**: Apply code changes to resolve real issues. Focus on:
   - Bugs and logic errors
   - Security vulnerabilities
   - Missing error handling
   - Race conditions
   - Resource leaks
3. **Skip false positives**: Do not change code for stylistic preferences or non-issues.
4. **Verify fixes**: After making changes, run relevant tests to confirm fixes don't break anything.

{{- if .AutoCommit }}

After fixing issues, stage and commit your changes:
```bash
git add -A && git commit -m "fix: address codex review findings"
```
{{- end }}

## Completion Signal

When ALL actionable findings have been addressed (or if there are no actionable findings), output exactly:

```
<<<PROGRAMMATOR:CODEX_REVIEW_DONE>>>
```

If there are still issues that need another Codex review pass, do NOT output the signal.
