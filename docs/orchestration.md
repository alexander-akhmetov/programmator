# Orchestration Flow

How programmator drives Claude Code through tasks and reviews, step by step.

## Commands

| Command | What it does |
|---------|--------------|
| `programmator start <ticket-or-plan>` | Run task phases, then review |
| `programmator review` | Run review only (no task phases) |
| `programmator plan create <desc>` | Interactive plan creation |

---

## 1. Task Execution (`programmator start`)

```
┌─────────────────────────────────────────────────────┐
│ Load config, detect source (ticket/plan), start TUI │
└──────────────────────┬──────────────────────────────┘
                       ▼
┌──────────────────────────────────────────────────────┐
│  ITERATION LOOP  (max_iterations, stagnation_limit)  │
│                                                      │
│  1. Fetch work item, find current unchecked phase    │
│  2. Build prompt:                                    │
│       Has phases? → phased.md                        │
│       No phases?  → phaseless.md                     │
│  3. Invoke: claude --print <prompt>                  │
│  4. Parse PROGRAMMATOR_STATUS from output             │
│  5. Mark phase complete, track files changed         │
│  6. Auto-commit if --auto-commit enabled             │
│  7. If status=CONTINUE and phases remain → loop      │
│     If status=DONE or all phases done → exit loop    │
│     If status=BLOCKED → abort                        │
└──────────────────────┬───────────────────────────────┘
                       ▼
           ┌───────────────────────┐
           │ All task phases done? │──no──→ (keep looping)
           └───────────┬───────────┘
                       │ yes
                       ▼
               (review flow below)
```

### Prompt: `phased.md`

Used when the work item has checkbox phases (`- [ ] Phase 1`, `- [ ] Phase 2`, ...).

**Template variables:** `{{.ID}}`, `{{.Title}}`, `{{.RawContent}}`, `{{.Notes}}`, `{{.CurrentPhase}}`, `{{.CurrentPhaseName}}`

Claude is told to work on **one phase per invocation**, run validation commands, then output:

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

Review runs automatically when all task phases complete.

```
┌─────────────────────────────────────────────────────┐
│ PHASE 1: "comprehensive" (1 iteration, all agents)  │
│                                                     │
│   Run configured agents in parallel (default: 6):   │
│   ┌──────────────┬───────────────┬────────────────┐ │
│   │ quality.md   │ security.md   │ implementation │ │
│   │ testing.md   │ simplific.md  │ linter.md      │ │
│   └──────────────┴───────────────┴────────────────┘ │
│                                                     │
│   Each agent: claude --print <agent prompt>         │
│   (prompt includes focus areas + changed files)     │
│   Agents output structured issues (severity, file,  │
│   line, description, fix suggestion)                │
│                                                     │
│   Issues found? → Invoke Claude with review_first.md│
│                   to fix them, then re-review       │
│   No issues?    → Advance to next phase             │
└──────────────────────┬──────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────┐
│ PHASE 2: "critical_loop" (10% of max_iterations)    │
│                                                     │
│   Run configured agents in parallel (default: 2):   │
│   ┌──────────────┬────────────────┐                 │
│   │ quality.md   │ implementation │                 │
│   └──────────────┴────────────────┘                 │
│                                                     │
│   Severity filter: only critical + high             │
│                                                     │
│   Issues found? → Invoke Claude with review_second.md│
│                   to fix, then re-review            │
│   No issues?    → Advance to next phase             │
└──────────────────────┬──────────────────────────────┘
                       ▼
┌─────────────────────────────────────────────────────┐
│ PHASE 3: "final_check" (10% of max_iterations)      │
│                                                     │
│   Same agents and filter as critical_loop           │
│   Final pass to catch anything introduced by fixes  │
│                                                     │
│   Passed? → All review phases complete → Done       │
│   Failed? → Exit with max_review_retries            │
└─────────────────────────────────────────────────────┘
```

### Prompt: `review_first.md`

Used for the **comprehensive** phase (no severity filter). Claude receives the full issue list from all 6 agents and is asked to fix everything.

**Template variables:** `{{.BaseBranch}}`, `{{.Iteration}}`, `{{.FilesList}}`, `{{.IssuesMarkdown}}`

### Prompt: `review_second.md`

Used for **filtered** phases (critical_loop, final_check). Claude receives only critical/high issues and focuses on correctness.

**Template variables:** same as review_first.md

### Review Agent Prompts

Each agent runs with its own embedded prompt from `internal/review/prompts/` by default.
You can override an agent prompt by setting `review.phases[].agents[].prompt` in config
(the prompt text is used directly).

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

Runs the same 3-phase review flow but without task phases. Operates on `git diff <base>...HEAD`.

```
1. Get changed files from git diff <base>...HEAD (default base: main)
2. Run all 3 review phases (same as above)
3. For each phase with issues:
   a. Build fix prompt (review_first.md or review_second.md)
   b. Invoke Claude to fix
   c. Auto-commit fixes
   d. Re-run review to verify
4. Print summary (passed/failed, issues, duration)
5. Exit code 0 (passed) or 1 (failed) for CI
```

---

## 4. Plan Creation (`programmator plan create`)

Interactive loop where Claude asks clarifying questions before generating a plan.

```
┌────────────────────────────────────────────────────┐
│ 1. Invoke Claude with plan_create.md               │
│    Variables: {{.Description}}, {{.PreviousAnswers}}│
│                                                    │
│ 2. Claude analyzes codebase, then either:          │
│    a. Asks a question:                             │
│       <<<PROGRAMMATOR:QUESTION>>>                   │
│       {"question": "...", "options": [...]}         │
│       <<<PROGRAMMATOR:END>>>                        │
│                                                    │
│    b. Outputs the plan:                            │
│       <<<PROGRAMMATOR:PLAN_READY>>>                 │
│       # Plan: Title                                │
│       ## Tasks                                     │
│       - [ ] Task 1                                 │
│       <<<PROGRAMMATOR:END>>>                        │
│                                                    │
│ 3. If question: show to user via fzf/numbered menu │
│    Append answer to PreviousAnswers, go to step 1  │
│                                                    │
│ 4. If plan ready: write to plans/ directory        │
└────────────────────────────────────────────────────┘
```

---

## Status Protocol

Task execution and review-fix invocations must end with this YAML block
(review agents output `REVIEW_RESULT`, not `PROGRAMMATOR_STATUS`):

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
(mirrored in `internal/review/config.go` for env-only defaults). Override via YAML config:

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
