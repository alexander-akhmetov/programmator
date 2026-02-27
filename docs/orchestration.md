# Orchestration Flow

A plain-language walkthrough of how programmator runs coding agents.

## Commands

| Command | What it does |
|---------|--------------|
| `programmator start <ticket-or-plan>` | Run task phases, then review |
| `programmator review` | Run review only (no task phases) |
| `programmator run <prompt>` | Run configured coding agent with a custom prompt (no plan/ticket) |
| `programmator config show` | Show resolved configuration |

---

## 1. Task Execution (`programmator start`)

1. Load config, detect source (ticket or plan), and start the CLI.
2. Loop until done or a safety exit triggers:
   - Read the work item and pick the first unchecked phase.
   - Choose a prompt template (phased.md with phases, phaseless.md without).
   - Invoke the configured executor with the prompt.
   - Parse PROGRAMMATOR_STATUS from the output.
   - Mark the phase complete and record changed files.
    - Auto-commit if --auto-commit is enabled.
    - Decide what to do next based on status:
      - CONTINUE keeps looping (as long as phases remain).
      - DONE marks the task complete and transitions to the review flow.
      - BLOCKED aborts the run.
3. After the last phase, continue to the review flow.
   - For phaseless work items, review begins after Claude returns status DONE.

### Prompt: `phased.md`

Used when the work item includes checkbox phases (`- [ ] Phase 1`, `- [ ] Phase 2`, ...).

**Template variables:** `{{.ID}}`, `{{.Title}}`, `{{.RawContent}}`, `{{.CurrentPhase}}`, `{{.CurrentPhaseName}}`

Claude is expected to work on **one phase per invocation**, run validation commands, then output:

```yaml
PROGRAMMATOR_STATUS:
  phase_completed: "Phase Name"
  status: CONTINUE
  files_changed: [...]
  summary: "..."
```

### Prompt: `phaseless.md`

Used when the work item has no phases (a simple ticket or description).

**Template variables:** `{{.ID}}`, `{{.Title}}`, `{{.RawContent}}`

Same status protocol but `phase_completed: null`.

**Note:** Progress notes are stored directly in the work item's `## Notes` section (included in `{{.RawContent}}`). Claude is instructed to append notes there.

---

## 2. Review Flow (after task phases)

Review runs automatically after all task phases complete. It uses a single loop with a flat list of agents.

1. Run all configured agents in parallel (default 9: bug-shallow, bug-deep, architect, simplification, silent-failures, claudemd, type-design, comments, tests-and-linters).
2. Each agent runs the configured executor with the agent prompt, focus areas, and changed files.
3. Agents return structured issues (severity, file, line, description, fix suggestion).
4. Optional validators run after primary agents (enabled by default):
   - **simplification-validator**: Filters low-value simplification suggestions.
   - **issue-validator**: Filters false positives from all other agents.
5. If issues remain, build a fix prompt using `review_first.md` and invoke the executor to fix them.
6. Auto-commit fixes if enabled.
7. Re-run the review (back to step 1) up to `review.max_iterations` times.
8. If no issues remain, review passes.

### Prompt: `review_first.md`

Claude receives the full issue list from all agents and is asked to fix everything.

**Template variables:** `{{.BaseBranch}}`, `{{.Iteration}}`, `{{.FilesList}}`, `{{.IssuesMarkdown}}`, `{{.AutoCommit}}`

### Review Agent Prompts

Each agent runs with its own embedded prompt from `internal/review/prompts/` by default.
You can override an agent prompt by setting either:
- `review.overrides[].prompt` / `review.agents[].prompt` (inline text)
- `review.overrides[].prompt_file` / `review.agents[].prompt_file` (file path)

| Agent | Prompt | Focus |
|-------|--------|-------|
| bug-shallow | `bug_shallow.md` | Obvious diff-visible bugs only |
| bug-deep | `bug_deep.md` | Context-aware bugs, security, leaks, concurrency |
| architect | `architect.md` | Architectural fit and coupling |
| simplification | `simplification.md` | Over-engineering, unnecessary abstractions |
| silent-failures | `silent_failures.md` | Swallowed errors, inadequate logging |
| claudemd | `claudemd.md` | CLAUDE.md compliance |
| type-design | `type_design.md` | Type/interface design quality |
| comments | `comments.md` | Comment accuracy and value |
| tests-and-linters | `linter.md` | Test failures, lint errors, formatting |

---

## 3. Review-Only Mode (`programmator review`)

Runs the same review loop but without task phases. It operates on `git diff <base>...HEAD`.

1. Get changed files from git diff <base>...HEAD (default base: main).
2. Run the review loop (same as above).
3. For each iteration with issues:
   - Build fix prompt using `review_first.md`.
   - Invoke Claude to fix.
   - Auto-commit fixes.
   - Re-run review to verify.
4. Print summary (passed/failed, issues, duration).
5. Exit code 0 (passed) or 1 (failed) for CI.

---

## Status Protocol

Task execution and review-fix invocations must end with this YAML block.
Review agents output `REVIEW_RESULT`, not `PROGRAMMATOR_STATUS`.

```yaml
PROGRAMMATOR_STATUS:
  phase_completed: "Phase Name" | null
  status: CONTINUE | DONE | BLOCKED
  files_changed:
    - file1.go
    - file2.go
  summary: "what was done"
  commit_made: true | false # optional (used by review-only auto-commit)
  error: "reason" # only if BLOCKED
```

| Status | Meaning |
|--------|---------|
| `CONTINUE` | More work needed, loop continues |
| `DONE` | All work complete |
| `BLOCKED` | Cannot proceed without human help |

---

## Configuration

Default agents and iteration limits are in `internal/config/defaults/config.yaml`.
Common review configuration patterns:

```yaml
# ~/.config/programmator/config.yaml or .programmator/config.yaml
review:
  max_iterations: 3
  parallel: true
  executor:
    name: claude
    claude:
      flags: "--model opus"

  # default mode (when review.agents is empty):
  include: [bug-shallow, bug-deep, architect, tests-and-linters, claudemd]
  exclude: [tests-and-linters]
  overrides:
    - name: bug-deep
      prompt_file: ".programmator/prompts/review/bug-deep.md"

  # custom mode (non-empty review.agents replaces defaults):
  # agents:
  #   - name: custom-review
  #     focus: [bugs, architecture]
  #     prompt_file: ".programmator/prompts/review/custom.md"

  validators:
    issue: true
    simplification: true
```

Prompt templates can be overridden per-project (`.programmator/prompts/`) or globally (`~/.config/programmator/prompts/`). See [prompt_templates.md](prompt_templates.md).
