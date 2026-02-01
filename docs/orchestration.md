# Orchestration Flow

A plain-language walkthrough of how programmator runs Claude Code.

## Commands

| Command | What it does |
|---------|--------------|
| `programmator start <ticket-or-plan>` | Run task phases, then review |
| `programmator review` | Run review only (no task phases) |
| `programmator run <prompt>` | Run Claude with a custom prompt (no plan/ticket) |
| `programmator plan create <desc>` | Interactive plan creation |
| `programmator status` | Show active sessions |
| `programmator logs [source-id]` | View execution logs (`--follow` to tail) |
| `programmator config show` | Show resolved configuration |

---

## 1. Task Execution (`programmator start`)

1. Load config, detect source (ticket or plan), and start the TUI.
2. Loop until done or a safety exit triggers:
   - Read the work item and pick the first unchecked phase.
   - Choose a prompt template (phased.md with phases, phaseless.md without).
   - Call `claude --print <prompt>`.
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

**Template variables:** `{{.ID}}`, `{{.Title}}`, `{{.RawContent}}`, `{{.Notes}}`, `{{.CurrentPhase}}`, `{{.CurrentPhaseName}}`

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

**Template variables:** `{{.ID}}`, `{{.Title}}`, `{{.RawContent}}`, `{{.Notes}}`

Same status protocol but `phase_completed: null`.

---

## 2. Review Flow (after task phases)

Review runs automatically after all task phases complete. It uses a single loop with a flat list of agents.

1. Run all configured agents in parallel (default 9: error-handling, logic, security, implementation, testing, simplification, linter, claudemd, codex).
2. Each agent runs `claude --print <agent prompt>` with focus areas and changed files.
3. Agents return structured issues (severity, file, line, description, fix suggestion).
4. Validators run automatically:
   - **simplification-validator**: Filters low-value simplification suggestions.
   - **issue-validator**: Filters false positives from all other agents.
5. If issues remain, build a fix prompt using `review_first.md` and invoke Claude to fix them.
6. Auto-commit fixes if enabled.
7. Re-run the review (back to step 1) up to `review.max_iterations` times.
8. If no issues remain, review passes.

### Prompt: `review_first.md`

Claude receives the full issue list from all agents and is asked to fix everything.

**Template variables:** `{{.BaseBranch}}`, `{{.Iteration}}`, `{{.FilesList}}`, `{{.IssuesMarkdown}}`, `{{.AutoCommit}}`

### Review Agent Prompts

Each agent runs with its own embedded prompt from `internal/review/prompts/` by default.
You can override an agent prompt by setting `review.agents[].prompt` in config.
The prompt text is used directly.

| Agent | Prompt | Focus |
|-------|--------|-------|
| error-handling | `quality.md` | Bugs, logic errors, race conditions, error handling, simplicity |
| logic | `quality.md` | Second quality pass for coverage |
| security | `security.md` | Injection, crypto, auth, data protection |
| implementation | `implementation.md` | Requirement coverage, wiring, completeness |
| testing | `testing.md` | Missing tests, fake tests, edge cases |
| simplification | `simplification.md` | Over-engineering, unnecessary abstractions |
| linter | `linter.md` | Auto-detect project type, run linters, report findings |
| claudemd | `claudemd.md` | CLAUDE.md accuracy and completeness |
| codex | `codex.md` | OpenAI Codex cross-check (skips if codex binary unavailable) |

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

## 4. Plan Creation (`programmator plan create`)

Interactive loop where Claude asks clarifying questions before generating a plan.

1. Invoke Claude with plan_create.md.
   - Variables: {{.Description}}, {{.PreviousAnswers}}
2. Claude analyzes codebase, then either:
   - Asks a question:
     - <<<PROGRAMMATOR:QUESTION>>>
     - {"question": "...", "options": [...]}
     - <<<PROGRAMMATOR:END>>>
   - Outputs the plan:
     - <<<PROGRAMMATOR:PLAN_READY>>>
     - # Plan: Title
     - ## Tasks
     - - [ ] Task 1
     - <<<PROGRAMMATOR:END>>>
3. If it's a question, show it to the user (fzf/numbered menu), append the answer to PreviousAnswers, and return to step 1.
4. If it's a plan, write it to the plans/ directory.

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

Default agents and iteration limits are in `internal/config/defaults/config.yaml`
(mirrored in `internal/review/config.go` for env-only defaults). Override them via YAML config:

```yaml
# ~/.config/programmator/config.yaml or .programmator/config.yaml
review:
  max_iterations: 3
  parallel: true
  agents:
    - name: error-handling
    - name: logic
    - name: security
    - name: implementation
    - name: testing
    - name: simplification
    - name: linter
    - name: claudemd
    - name: codex
```

Prompt templates can be overridden per-project (`.programmator/prompts/`) or globally (`~/.config/programmator/prompts/`). See [prompt_templates.md](prompt_templates.md).
