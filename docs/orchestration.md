# Orchestration Flow

A plain-language walkthrough of how programmator runs Claude Code.

## Commands

| Command | What it does |
|---------|--------------|
| `programmator start <ticket-or-plan>` | Run task phases, then review |
| `programmator review` | Run review only (no task phases) |
| `programmator plan create <desc>` | Interactive plan creation |

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
     - DONE exits (or no phases remain).
     - BLOCKED aborts the run.
3. After the last phase, continue to the review flow.

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

Review runs automatically after all task phases complete.

1. Phase 1: "comprehensive" (1 iteration, all agents)
   - Run configured agents in parallel (default: 6): quality, security, implementation, testing, simplification, linter.
   - Each agent runs `claude --print <agent prompt>` with focus areas and changed files.
   - Agents return structured issues (severity, file, line, description, fix suggestion).
   - If issues are found, run review_first.md to fix them, then re-run the phase.
   - If no issues are found, move on.
2. Phase 2: "critical_loop" (10% of max_iterations)
   - Run configured agents in parallel (default: 2): quality, implementation.
   - Severity filter: critical + high only.
   - If issues are found, run review_second.md to fix them, then re-run the phase.
   - If no issues are found, move on.
3. Phase 3: "final_check" (10% of max_iterations)
   - Same agents and severity filter as critical_loop.
   - Final pass to catch anything introduced by fixes.
   - If passed, all review phases are complete.
   - If failed, exit with max_review_retries.

### Prompt: `review_first.md`

Used for the **comprehensive** phase (no severity filter). Claude receives the full issue list from all 6 agents and is asked to fix everything.

**Template variables:** `{{.BaseBranch}}`, `{{.Iteration}}`, `{{.FilesList}}`, `{{.IssuesMarkdown}}`

### Prompt: `review_second.md`

Used for **filtered** phases (critical_loop, final_check). Claude receives only critical/high issues and focuses on correctness.

**Template variables:** same as review_first.md

### Review Agent Prompts

Each agent runs with its own embedded prompt from `internal/review/prompts/` by default.
You can override an agent prompt by setting `review.phases[].agents[].prompt` in config.
The prompt text is used directly.

| Agent | Prompt | Focus |
|-------|--------|-------|
| quality | `quality.md` | Bugs, logic errors, race conditions, error handling, simplicity |
| security | `security.md` | Injection, crypto, auth, data protection |
| implementation | `implementation.md` | Requirement coverage, wiring, completeness |
| testing | `testing.md` | Missing tests, fake tests, edge cases |
| simplification | `simplification.md` | Over-engineering, unnecessary abstractions |
| linter | `linter.md` | Auto-detect project type, run linters, report findings |

---

## 3. Review-Only Mode (`programmator review`)

Runs the same 3-phase review flow but without task phases. It operates on `git diff <base>...HEAD`.

1. Get changed files from git diff <base>...HEAD (default base: main).
2. Run all 3 review phases (same as above).
3. For each phase with issues:
   - Build fix prompt (review_first.md or review_second.md).
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

Default review phases, iteration limits, and agents are in `internal/config/defaults/config.yaml`
(mirrored in `internal/review/config.go` for env-only defaults). Override them via YAML config:

```yaml
# ~/.config/programmator/config.yaml or .programmator/config.yaml
review:
  max_iterations: 50
  phases:
    - name: comprehensive
      iteration_limit: 1
      parallel: true
      agents:
        - name: quality
        - name: security
        - name: implementation
        - name: testing
        - name: simplification
        - name: linter
    - name: critical_loop
      iteration_pct: 10
      severity_filter: [critical, high]
      parallel: true
      agents:
        - name: quality
        - name: implementation
    - name: final_check
      iteration_pct: 10
      severity_filter: [critical, high]
      parallel: true
      agents:
        - name: quality
        - name: implementation
```

Prompt templates can be overridden per-project (`.programmator/prompts/`) or globally (`~/.config/programmator/prompts/`). See [prompt_templates.md](prompt_templates.md).
