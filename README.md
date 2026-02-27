# Programmator

[![CI](https://github.com/alexander-akhmetov/programmator/actions/workflows/ci.yml/badge.svg)](https://github.com/alexander-akhmetov/programmator/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/alexander-akhmetov/programmator)](https://goreportcard.com/report/github.com/alexander-akhmetov/programmator)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Autonomous coding agent orchestrator that executes multi-task plans without supervision. Supports [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [pi coding agent](https://github.com/badlogic/pi-mono) as executors.

Coding agents are interactive — they require you to watch, approve, and guide each step. For complex features spanning multiple tasks, this means hours of babysitting. As context fills up during long sessions, the model starts making mistakes and producing worse code.

Programmator splits work into isolated sessions with fresh context windows. Each task runs independently, gets reviewed by parallel agents, and can be auto-committed on completion with no supervision needed.

## Quick Start

**Requirements:**
- Go 1.26.0+
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI or [pi coding agent](https://github.com/badlogic/pi-mono) CLI

```bash
go install github.com/alexander-akhmetov/programmator/cmd/programmator@latest
```

Write a plan file (`plan.md`):
```markdown
# Plan: Fix calculator bugs

## Validation Commands
- `go test ./...`

## Tasks
- [ ] Fix add() to return a + b instead of a - b
- [ ] Fix off-by-one error in loop
- [ ] Add missing nil check in User handler
```

Run it:
```bash
programmator start ./plan.md
```

Programmator picks up the first unchecked task, invokes Claude Code to complete it, marks it done, and moves to the next. After all tasks complete, it runs a multi-agent code review. When everything passes (or safety limits are hit), it stops.

## How It Works

Each iteration:
1. Reads the plan file and finds the first uncompleted task
2. Builds a prompt with the task context and instructions
3. Invokes Claude Code in a fresh session
4. Parses Claude's `PROGRAMMATOR_STATUS` output block (YAML with status, files changed, summary)
5. Updates the task checkbox and logs progress
6. Checks safety limits, then loops back

After all tasks are done, a [multi-agent review](#review) runs automatically.

## Plan

When you run `programmator start <thing>`, the source type is auto-detected from the argument: file paths → plan file, everything else → ticket ID.

### Tickets

If you have [ticket](https://github.com/wedow/ticket) CLI installed, programmator can use it to get the plan from the ticket. Tickets are markdown files with YAML frontmatter and checkbox phases.

### Files

A plan file is a markdown file with checkbox tasks:

```markdown
# Plan: Feature Name

## Validation Commands
- `go test ./...`
- `golangci-lint run`

## Tasks
- [x] Task 1: Investigate current implementation and update plan.md (already completed, will be skipped)
- [ ] Task 2: Implement the feature
- [ ] Task 3: Add tests
- [ ] Task 4: Cleanup
```

- **Title**: First `# ` heading (optional `Plan:` prefix)
- **Validation Commands**: Run after each task completion (optional)
- **Tasks**: Checkbox items (`- [ ]` / `- [x]`) anywhere in the file

Create plans interactively — the executor analyzes your codebase and asks clarifying questions:

```bash
programmator plan create "Add authentication to the API"
```

## Review

After all tasks complete, programmator automatically runs a multi-agent code review. 9 agents run in parallel — each focused on a specific area (bug detection, architecture, simplification, silent failures, CLAUDE.md compliance, type design, comments, and tests/linting). Issues found are auto-fixed and re-reviewed, up to 3 iterations.

You can also run review standalone on any branch:

```bash
programmator review                       # review current branch vs main
programmator review --base develop        # review against a different base
```

## Commands

```bash
programmator start ./plan.md              # execute a plan
programmator start ./plan.md --auto-commit # with git workflow (branch + commits)
programmator start pro-1a2b               # execute a ticket
programmator review                       # review-only mode on current branch
programmator run "explain this codebase"  # run Claude with a custom prompt
programmator plan create "description"    # interactive plan creation
programmator status                       # show active sessions
programmator logs --follow                # tail the active log
programmator config show                  # show resolved config
```

`programmator run` is a lightweight wrapper around the configured coding agent — pass any prompt as an argument or pipe via stdin. Useful for one-off tasks that don't need plan tracking.

## Safety Gates

- **Guard mode**: If [dcg](https://github.com/Dicklesworthstone/destructive_command_guard) is installed, programmator uses it to block destructive shell commands during autonomous execution.
- **Max iterations**: Prevents runaway loops (default: 50)
- **Stagnation detection**: Exits if no files change for N iterations (default: 3)
- **Error repetition**: Exits if same error occurs 3 times
- **Timeout**: Kills Claude if a single invocation takes too long (default: 900s)
- **Ctrl+C**: Graceful stop after current iteration

## Auto Git Workflow

Opt-in via config or CLI flags:
- `--auto-commit`: Creates a `programmator/<slug>` branch, commits after each phase
- `--move-completed`: Moves completed plans to `plans/completed/`
- `--branch [optional name]`: Custom branch name

## Configuration

Programmator uses a unified YAML config with multi-level merge (highest priority last):

1. [Embedded defaults](internal/config/defaults/config.yaml) (built into binary)
2. Global config (`~/.config/programmator/config.yaml`)
3. Environment variables
4. Local config (`.programmator/config.yaml` in project directory)
5. CLI flags

See resolved values with `programmator config show`.

<details>
<summary>Config keys</summary>

| Key | Default | Description |
|-----|---------|-------------|
| `max_iterations` | `50` | Maximum loop iterations before forced exit |
| `stagnation_limit` | `3` | Exit after N consecutive iterations with no file changes |
| `timeout` | `900` | Seconds per executor invocation |
| `executor` | `claude` | Which coding agent to use (`"claude"` or `"pi"`) |
| `claude.flags` | `""` | Additional flags passed to the `claude` command |
| `claude.config_dir` | `""` | Custom Claude config directory (empty = default) |
| `claude.anthropic_api_key` | `""` | Anthropic API key passed to Claude (overrides env) |
| `pi.flags` | `""` | Additional flags passed to the `pi-coding-agent` command |
| `pi.config_dir` | `""` | Custom PI_CODING_AGENT_DIR (empty = default) |
| `pi.provider` | `""` | LLM provider for pi (e.g. `"anthropic"`, `"openai"`) |
| `pi.model` | `""` | Model name for pi (e.g. `"sonnet"`, `"gpt-4o"`) |
| `pi.api_key` | `""` | API key for the configured pi provider |
| `ticket_command` | `tk` | Binary name for the ticket CLI (`tk` or `ticket`) |
| `git.auto_commit` | `false` | Auto-commit after each phase completion |
| `git.move_completed_plans` | `false` | Move completed plans to a `completed/` directory |
| `git.completed_plans_dir` | `""` | Directory for completed plans (default: `plans/completed`) |
| `git.branch_prefix` | `""` | Prefix for auto-created branches (default: `programmator/`) |
| `review.max_iterations` | `3` | Maximum review fix iterations |
| `review.parallel` | `true` | Run review agents in parallel |
| `review.agents` | see [defaults](internal/config/defaults/config.yaml) | Flat list of review agents with names and focus areas |

</details>

<details>
<summary>Environment variables</summary>

Environment variables used by programmator and its executors:

| Variable | Default | Description |
|----------|---------|-------------|
| `PROGRAMMATOR_DEBUG` | `""` | Set to `1` to enable debug output |
| `PROGRAMMATOR_STATE_DIR` | XDG state dir | Override the state directory path |
| `TICKETS_DIR` | `~/.tickets` | Where ticket files live |
| `CLAUDE_CONFIG_DIR` | - | Custom Claude config directory (passed to Claude subprocess) |
| `PI_CODING_AGENT_DIR` | - | Custom pi coding agent config directory |

</details>

<details>
<summary>Prompt templates</summary>

Prompts are customizable via Go `text/template` files. Override any prompt by placing a file in:
- `~/.config/programmator/prompts/` (global)
- `.programmator/prompts/` (per-project)

Available templates: `phased.md`, `phaseless.md`, `review_first.md`, `plan_create.md`. See [prompt template docs](docs/prompt_templates.md) for variables and examples.

</details>

## Claude Code Plugin

Programmator ships a Claude Code plugin with commands for converting plans between formats.

### Installation

```bash
# Add the marketplace (one-time)
/plugin marketplace add alexander-akhmetov/programmator

# Install the plugin
/plugin install programmator@alexander-akhmetov-programmator
```

### Commands

- **`/plan-to-ticket`** — Reads the most recent Claude Code plan (`~/.claude/plans/*.md`), extracts phases, and creates a programmator ticket via the `ticket` CLI.
- **`/plan-to-file`** — Reads the most recent Claude Code plan and converts it into a programmator-compatible plan file (`plan.md`) in the current directory, ready for `programmator start ./plan.md`.

## Documentation

- [Orchestration flow](docs/orchestration.md) — detailed walkthrough of execution, review, and plan creation
- [Prompt templates](docs/prompt_templates.md) — override chain, template variables, customization
- [E2E tests](docs/e2e_tests.md) — manual integration tests

## Development

```bash
go build ./...                # Build
go test ./...                 # Run tests
go test -race ./...           # Run tests with race detector
golangci-lint run             # Lint

# E2E test prep (creates toy projects in /tmp)
make e2e-prep                 # Plan-based run
make e2e-review               # Review mode
make e2e-plan                 # Interactive plan creation
```

## Releasing

Push a git tag to trigger a GitHub Actions release via GoReleaser:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Binaries are published for linux/darwin (amd64/arm64).
