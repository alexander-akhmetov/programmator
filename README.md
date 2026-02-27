# Programmator

[![CI](https://github.com/alexander-akhmetov/programmator/actions/workflows/ci.yml/badge.svg)](https://github.com/alexander-akhmetov/programmator/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/alexander-akhmetov/programmator)](https://goreportcard.com/report/github.com/alexander-akhmetov/programmator)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Autonomous coding agent orchestrator that executes multi-task plans without supervision. Supports [Claude Code](https://docs.anthropic.com/en/docs/claude-code) and [pi coding agent](https://github.com/badlogic/pi-mono) as executors.

Coding agents are interactive — they require you to watch, approve, and guide each step. For complex features spanning multiple tasks, this means hours of babysitting. As context fills up during long sessions, the model starts making mistakes and producing worse code.

Programmator splits work into isolated sessions with fresh context windows. Each task runs independently, gets reviewed by parallel agents, and can be auto-committed on completion with no supervision needed.

## Install with Claude Code

The plugin can help you install and configure programmator. For manual installation without the plugin, see [Manual Install](#manual-install).

**1. Install the plugin:**

```bash
claude plugin marketplace add alexander-akhmetov/programmator
claude plugin install -s user programmator
```

**2. Ask Claude to install and configure programmator:**

Claude will help you install the binary, verify your executor is available, and optionally create a config file. Just ask — for example: *"install and configure programmator"*.

**3. Use it:**

Create plans manually or use `/plan-to-file` to convert Claude Code plans. Then run `programmator start ./plan.md`.

## Manual Install

Download a binary from [GitHub Releases](https://github.com/alexander-akhmetov/programmator/releases), or install with Go (requires 1.26+):

```bash
go install github.com/alexander-akhmetov/programmator/cmd/programmator@latest
```

You'll also need at least one executor: [Claude Code](https://docs.anthropic.com/en/docs/claude-code) or [pi coding agent](https://github.com/badlogic/pi-mono).

## Quick Start

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

## Examples

### Use pi coding agent instead of Claude Code

Create `~/.config/programmator/config.yaml`:

```yaml
executor: pi
pi:
  provider: anthropic # optional
  model: sonnet       # optional
```

Then `programmator start ./plan.md` uses pi for all tasks instead of Claude Code.

### Minimal config: your own executor + one custom review agent

By default, programmator runs 9 review agents after task completion. You can replace them all with a single custom one:

```yaml
executor: pi

review:
  agents:
    - name: code-review
      focus:
        - correctness
        - error handling
        - test coverage
```

This runs pi for coding and a single `code-review` agent for the final review pass. You can also point to a full custom prompt file:

```yaml
review:
  agents:
    - name: code-review
      prompt_file: ".programmator/prompts/review/code-review.md"
```

And in `.programmator/prompts/review/code-review.md`:

``` markdown
Do a good code review please.
```

### Code with pi, review with Claude Opus

```yaml
executor: pi

review:
  executor:
    name: claude
    claude:
      flags: "--model opus"
```

## How It Works

Each iteration:
1. Reads the plan file and finds the first uncompleted task
2. Builds a prompt with the task context and instructions
3. Invokes the configured executor in a fresh session
4. Parses the executor's `PROGRAMMATOR_STATUS` output block (YAML with status, files changed, summary)
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

## Review

After all tasks complete, programmator automatically runs a multi-agent code review. By default 9 agents run in parallel (bug-shallow, bug-deep, architect, simplification, silent-failures, claudemd, type-design, comments, tests-and-linters). Issues found are auto-fixed and re-reviewed, up to 3 iterations.

Review configuration is flexible:
- Use the default 9 agents
- Select a subset with `review.include` / `review.exclude`
- Override prompts/focus for default agents with `review.overrides`
- Replace defaults entirely with a custom `review.agents` list
- Use a different executor/model for review via `review.executor`

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
programmator run "explain this codebase"  # run configured coding agent with a custom prompt
programmator config show                  # show resolved config
```

`programmator run` is a lightweight wrapper around the configured coding agent — pass any prompt as an argument or pipe via stdin. Useful for one-off tasks that don't need plan tracking.

## Safety Gates

- **Guard mode**: If [dcg](https://github.com/Dicklesworthstone/destructive_command_guard) is installed, programmator uses it to block destructive shell commands during autonomous execution.
- **Max iterations**: Prevents runaway loops (default: 50)
- **Stagnation detection**: Exits if no files change for N iterations (default: 3)
- **Error repetition**: Exits if same error occurs 3 times
- **Timeout**: Kills the executor if a single invocation takes too long (default: 900s)
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
3. Local config (`.programmator/config.yaml` in project directory)
4. CLI flags

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
| `review.executor.name` | `""` | Optional review executor override (`claude` / `pi`, empty = inherit top-level) |
| `review.executor.claude.flags` | `""` | Review-only Claude flags (for example `--model opus`) |
| `review.executor.pi.*` | `""` | Review-only PI settings (`flags`, `config_dir`, `provider`, `model`, `api_key`) |
| `review.include` | `[]` | Subset of built-in review agents (empty = all defaults) |
| `review.exclude` | `[]` | Remove specific default review agents |
| `review.overrides` | `[]` | Override default agents by name (focus/prompt/prompt_file) |
| `review.agents` | `[]` | Explicit custom review agents; when non-empty replaces defaults |
| `review.validators.issue` | `true` | Run cross-agent false-positive validator |
| `review.validators.simplification` | `true` | Run simplification value validator |

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

Available templates: `phased.md`, `phaseless.md`, `review_first.md`. See [prompt template docs](docs/prompt_templates.md) for variables and examples.

</details>

## Claude Code Plugin

The plugin (see [Install with Claude Code](#install-with-claude-code)) provides:

- **`/programmator-setup`** — Guided install and configuration of programmator.
- **`/plan-to-file`** — Convert the most recent Claude Code plan into a programmator plan file (`plan.md`), ready for `programmator start ./plan.md`.
- **`/plan-to-ticket`** — Convert the most recent Claude Code plan into a programmator ticket (requires `ticket` CLI).

## Documentation

- [Orchestration flow](docs/orchestration.md) — detailed walkthrough of execution and review
- [Prompt templates](docs/prompt_templates.md) — override chain, template variables, customization
- [E2E tests](docs/e2e_tests.md) — manual integration tests

## Development

```bash
go build ./...                # Build
go test ./...                 # Run tests
go test -race ./...           # Run tests with race detector
make lint                     # Lint (golangci-lint + govulncheck + deadcode + go mod tidy)

# E2E test prep (creates toy projects in /tmp)
make e2e-prep                 # Plan-based run
make e2e-review               # Review mode
```

## Releasing

Push a git tag to trigger a GitHub Actions release via GoReleaser:

```bash
git tag v1.0.0
git push origin v1.0.0
```

Binaries are published for linux/darwin (amd64/arm64).
