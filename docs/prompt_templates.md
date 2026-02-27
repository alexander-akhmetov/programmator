# Prompt Templates

Programmator renders prompts from Go `text/template` files and sends them to the configured coding agent. You can override any template globally or per project.

## Templates

| Template | Used When |
|----------|-----------|
| [phased.md](../internal/config/defaults/prompts/phased.md) | Work item has checkbox phases |
| [phaseless.md](../internal/config/defaults/prompts/phaseless.md) | Work item has no phases (single task) |
| [review_first.md](../internal/config/defaults/prompts/review_first.md) | Review fix prompt (issues found by agents) |

## Override Order

Templates resolve in this order (first match wins):

```
.programmator/prompts/<name>.md          # Local (project-specific)
~/.config/programmator/prompts/<name>.md # Global (all projects)
embedded defaults                        # Built into the binary
```

Only create the templates you want to override. Missing files fall through to the next level.

## Template Variables

### phased.md / phaseless.md

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ID}}` | string | Work item identifier (ticket ID or plan filename) |
| `{{.Title}}` | string | Human-readable title |
| `{{.RawContent}}` | string | Full content of the work item (includes `## Notes` section if present) |
| `{{.CurrentPhase}}` | string | Current phase name, or "All phases complete" *(phased only)* |
| `{{.CurrentPhaseName}}` | string | Raw phase name for the status block, or "null" *(phased only)* |

**Note:** Progress notes are stored in the `## Notes` section within the work item itself, so they appear in `{{.RawContent}}`. The prompt template instructs Claude to append notes to this section.

### review_first.md

| Variable | Type | Description |
|----------|------|-------------|
| `{{.BaseBranch}}` | string | Base branch for comparison |
| `{{.Iteration}}` | int | Current review iteration number |
| `{{.FilesList}}` | string | Formatted list of files to review |
| `{{.IssuesMarkdown}}` | string | Markdown-formatted issues to fix |
| `{{.AutoCommit}}` | bool | Whether auto-commit is enabled |

## Creating an Override

1. Pick the scope (global or local):

```bash
# Global (applies to all projects)
mkdir -p ~/.config/programmator/prompts

# Local (applies to this project only)
mkdir -p .programmator/prompts
```

2. Copy a default template as a starting point. Defaults live in `internal/config/defaults/prompts/`. For example:

```bash
cp internal/config/defaults/prompts/phased.md ~/.config/programmator/prompts/phased.md
```

3. Edit the copy. Use any Go `text/template` syntax. Lines starting with `#` are stripped automatically (treated as comments, not markdown headings).

4. Run programmator. It picks up the override automatically; no config changes needed.

## Comment Syntax

Any line whose first non-whitespace character is `#` is stripped before parsing. This lets you add notes to your templates:

```markdown
# This line is stripped (comment)
## This line is also stripped (starts with #)
You are working on {{.ID}}
Use a # inline and it stays
```

To keep a markdown heading, indent it or structure the line so it doesn't start with `#`. The default templates intentionally use `#`/`##` lines as comments for readability, so those headings are stripped from the final prompt. If you need headings in the actual prompt, use a different marker (for example, `===` or bold text).

## Example: Custom Phased Template

```markdown
# My custom phased prompt
# Comments here are stripped

You are an autonomous coding agent working on: {{.Title}}

Task:
{{.RawContent}}

Focus on:
{{.CurrentPhase}}

IMPORTANT: Append notes to the "## Notes" section of the plan/ticket file.
Include: progress updates, key decisions, important findings, blockers.

When done, output this block:

PROGRAMMATOR_STATUS:
  phase_completed: "{{.CurrentPhaseName}}"
  status: CONTINUE
  files_changed:
    - file.go
  summary: "what you did"
```

## Review Agent Prompts

The review system uses embedded prompts in `internal/review/prompts/` by default.
You can override prompts with:
- `review.overrides[].prompt` or `review.agents[].prompt` (inline text)
- `review.overrides[].prompt_file` or `review.agents[].prompt_file` (file path, relative paths resolved from working directory)

When `review.agents` is non-empty, it replaces default agents.
When `review.agents` is empty, defaults are used and can be filtered by `review.include` / `review.exclude`.

| Agent | Prompt | Focus |
|-------|--------|-------|
| bug-shallow | [bug_shallow.md](../internal/review/prompts/bug_shallow.md) | Obvious bugs in diff only |
| bug-deep | [bug_deep.md](../internal/review/prompts/bug_deep.md) | Context-aware bugs, security, leaks, concurrency |
| architect | [architect.md](../internal/review/prompts/architect.md) | Architectural fit, alternatives, coupling |
| simplification | [simplification.md](../internal/review/prompts/simplification.md) | Over-engineering, unnecessary abstractions |
| silent-failures | [silent_failures.md](../internal/review/prompts/silent_failures.md) | Swallowed errors, missing logging |
| claudemd | [claudemd.md](../internal/review/prompts/claudemd.md) | CLAUDE.md compliance |
| type-design | [type_design.md](../internal/review/prompts/type_design.md) | Type/interface design quality |
| comments | [comments.md](../internal/review/prompts/comments.md) | Comment/doc accuracy |
| tests-and-linters | [linter.md](../internal/review/prompts/linter.md) | Tests, linters, formatting |
