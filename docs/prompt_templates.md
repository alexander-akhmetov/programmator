# Prompt Templates

Programmator uses Go `text/template` files to build the prompts sent to Claude Code. You can override any template globally or per-project.

## Templates

| Template | Used When |
|----------|-----------|
| [phased.md](../internal/config/defaults/prompts/phased.md) | Work item has checkbox phases |
| [phaseless.md](../internal/config/defaults/prompts/phaseless.md) | Work item has no phases (single task) |
| [review_first.md](../internal/config/defaults/prompts/review_first.md) | First (comprehensive) review phase |
| [review_second.md](../internal/config/defaults/prompts/review_second.md) | Critical/major issues review phase |
| [plan_create.md](../internal/config/defaults/prompts/plan_create.md) | Interactive plan creation |

## Override Chain

Templates resolve in this order (first match wins):

```
.programmator/prompts/<name>.md          # Local (project-specific)
~/.config/programmator/prompts/<name>.md # Global (all projects)
embedded defaults                        # Built into the binary
```

You only need to create the files you want to override. Missing files fall through to the next level.

## Template Variables

### phased.md / phaseless.md

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ID}}` | string | Work item identifier (ticket ID or plan filename) |
| `{{.Title}}` | string | Human-readable title |
| `{{.RawContent}}` | string | Full content of the work item |
| `{{.Notes}}` | string | Formatted progress notes from previous iterations |
| `{{.CurrentPhase}}` | string | Current phase name, or "All phases complete" *(phased only)* |
| `{{.CurrentPhaseName}}` | string | Raw phase name for the status block, or "null" *(phased only)* |

### review_first.md / review_second.md

| Variable | Type | Description |
|----------|------|-------------|
| `{{.BaseBranch}}` | string | Base branch for comparison |
| `{{.Iteration}}` | int | Current review iteration number |
| `{{.FilesList}}` | string | Formatted list of files to review |
| `{{.IssuesMarkdown}}` | string | Markdown-formatted issues to fix |

### plan_create.md

| Variable | Type | Description |
|----------|------|-------------|
| `{{.Description}}` | string | User's description of what to accomplish |
| `{{.PreviousAnswers}}` | string | Formatted Q&A from previous interactions (may be empty) |

Use `{{if .PreviousAnswers}}...{{end}}` to conditionally render the previous answers section.

## Creating an Override

1. Pick the scope — global or local:

```bash
# Global (applies to all projects)
mkdir -p ~/.config/programmator/prompts

# Local (applies to this project only)
mkdir -p .programmator/prompts
```

2. Copy the default template as a starting point. Defaults live in `internal/config/defaults/prompts/`. For example:

```bash
cp internal/config/defaults/prompts/phased.md ~/.config/programmator/prompts/phased.md
```

3. Edit the copy. Use any Go `text/template` syntax. Lines starting with `#` are stripped automatically (treated as comments, not markdown headings).

4. Run programmator — it picks up the override automatically. No config changes needed.

## Comment Syntax

Lines where the first non-whitespace character is `#` are stripped before parsing. This lets you add notes to your templates:

```markdown
# This line is stripped (comment)
## This line is also stripped (starts with #)
You are working on {{.ID}}
Use a # inline and it stays
```

To keep a markdown heading, indent it or restructure so it doesn't start the line with `#`. The default templates intentionally use `#`/`##` lines as comments for readability, so those headings are stripped from the final prompt. If you need headings in the actual prompt, use a different marker (for example, `===` or bold text).

## Example: Custom Phased Template

```markdown
# My custom phased prompt
# Comments here are stripped

You are an autonomous coding agent working on: {{.Title}}

Task:
{{.RawContent}}

What happened so far:
{{.Notes}}

Focus on:
{{.CurrentPhase}}

When done, output this block:

PROGRAMMATOR_STATUS:
  phase_completed: "{{.CurrentPhaseName}}"
  status: CONTINUE
  files_changed:
    - file.go
  summary: "what you did"
```

## Review Agent Prompts

The review system uses separate embedded prompts in `internal/review/prompts/` by default.
You can override an agent prompt by setting `review.phases[].agents[].prompt` in config
(the prompt text is used directly; it is not loaded from a file).

| Agent | Prompt | Focus |
|-------|--------|-------|
| quality | [quality.md](../internal/review/prompts/quality.md) | Bugs, logic errors, race conditions, error handling |
| security | [security.md](../internal/review/prompts/security.md) | Injection, crypto, auth, data protection |
| implementation | [implementation.md](../internal/review/prompts/implementation.md) | Requirement coverage, wiring, completeness |
| testing | [testing.md](../internal/review/prompts/testing.md) | Missing tests, fake tests, edge cases |
| simplification | [simplification.md](../internal/review/prompts/simplification.md) | Over-engineering, unnecessary abstractions |
| linter | [linter.md](../internal/review/prompts/linter.md) | Auto-detect project type, run linters, report findings |
