---
name: programmator-setup
description: "Install and configure programmator — autonomous coding agent orchestrator. Use when the user asks to install programmator, set up configuration, or get started."
allowed-tools: Bash(programmator:*) Bash(go:*) Bash(mkdir:*) Read Write AskUserQuestion
---

# Programmator Setup

Help the user install and configure programmator.

## Installation

Requires Go 1.26+.

```bash
go install github.com/alexander-akhmetov/programmator/cmd/programmator@latest
```

Or download a binary from [GitHub Releases](https://github.com/alexander-akhmetov/programmator/releases) for linux/darwin (amd64/arm64).

### Setup steps

1. **Install the binary** (see above). Verify: `programmator --help`
2. **Check that an executor is available** — programmator needs a coding agent to do the work:
   - **Claude Code** (default): `claude --version`
   - **pi coding agent**: `pi-coding-agent --version`
   - **OpenCode**: `opencode --version`
   - **Codex**: `codex --version`
   If none are installed, tell the user they need at least one.
3. **Optionally create a config** — programmator works with zero config (defaults to Claude Code as executor). Only needed if the user wants to use a different executor, customize review agents, or enable auto-commit. Config goes in `~/.config/programmator/config.yaml` (global) or `.programmator/config.yaml` (per-project override).
4. **Try it out** — create a simple plan file and run it:

```bash
programmator start ./plan.md
```

## Configuration

Programmator works with zero config. It defaults to Claude Code as executor with 9 parallel review agents. Only create a config if you need to customize something.

Config file locations (highest priority last):
1. Embedded defaults (built into binary)
2. `~/.config/programmator/config.yaml` (global)
3. `.programmator/config.yaml` (per-project)
4. CLI flags

Run `programmator config show` to see all resolved values and their sources.

### Use pi coding agent instead of Claude Code

```yaml
executor: pi
pi:
  provider: anthropic   # or "openai"
  model: sonnet          # or "gpt-4o"
```

### Custom review agents

Default: 9 agents run in parallel (bug-shallow, bug-deep, architect, simplification, silent-failures, claudemd, type-design, comments, tests-and-linters). You can replace them:

```yaml
review:
  agents:
    - name: code-review
      focus:
        - correctness
        - error handling
        - test coverage
```

Or point to a full prompt file:

```yaml
review:
  agents:
    - name: code-review
      prompt_file: ".programmator/prompts/review/code-review.md"
```

Or select a subset of defaults:

```yaml
review:
  include:
    - bug-deep
    - architect
    - tests-and-linters
```

Or exclude specific agents:

```yaml
review:
  exclude:
    - comments
    - type-design
```

### Use a different executor for review

Code with pi, review with Claude Opus:

```yaml
executor: pi
pi:
  provider: anthropic
  model: sonnet

review:
  executor:
    name: claude
    claude:
      flags: "--model opus"
```

### Auto-commit after each task

```yaml
git:
  auto_commit: true
```

Or via CLI: `programmator start ./plan.md --auto-commit`

### Config keys

| Key | Default | Description |
|-----|---------|-------------|
| `max_iterations` | `50` | Maximum loop iterations before forced exit |
| `stagnation_limit` | `3` | Exit after N consecutive iterations with no file changes |
| `timeout` | `900` | Seconds per executor invocation |
| `executor` | `claude` | Which coding agent to use (`"claude"`, `"pi"`, `"opencode"`, or `"codex"`) |
| `claude.flags` | `""` | Additional flags passed to the `claude` command |
| `claude.config_dir` | `""` | Custom Claude config directory |
| `claude.anthropic_api_key` | `""` | Anthropic API key (overrides env) |
| `pi.flags` | `""` | Additional flags passed to `pi-coding-agent` |
| `pi.config_dir` | `""` | Custom PI_CODING_AGENT_DIR |
| `pi.provider` | `""` | LLM provider for pi (`"anthropic"`, `"openai"`) |
| `pi.model` | `""` | Model name for pi (`"sonnet"`, `"gpt-4o"`) |
| `pi.api_key` | `""` | API key for the configured pi provider |
| `opencode.flags` | `""` | Additional flags passed to `opencode` |
| `opencode.config_dir` | `""` | Custom OPENCODE_CONFIG_DIR |
| `opencode.model` | `""` | Model in `"provider/model"` format |
| `opencode.api_key` | `""` | API key for the configured provider |
| `codex.flags` | `""` | Additional flags passed to `codex` |
| `codex.model` | `""` | Model name (e.g. `"o3"`, `"gpt-5-codex"`) |
| `codex.api_key` | `""` | OpenAI API key |
| `ticket_command` | `tk` | Binary name for the ticket CLI |
| `git.auto_commit` | `false` | Auto-commit after each phase completion |
| `git.move_completed_plans` | `false` | Move completed plans to a `completed/` directory |
| `git.completed_plans_dir` | `""` | Directory for completed plans (default: `plans/completed`) |
| `git.branch_prefix` | `""` | Prefix for auto-created branches (default: `programmator/`) |
| `review.max_iterations` | `3` | Maximum review fix iterations |
| `review.parallel` | `true` | Run review agents in parallel |
| `review.include` | `[]` | Subset of built-in review agents |
| `review.exclude` | `[]` | Remove specific default review agents |
| `review.agents` | `[]` | Custom review agents (replaces defaults when non-empty) |
| `review.validators.issue` | `true` | Cross-agent false-positive validator |
| `review.validators.simplification` | `true` | Simplification value validator |

## Plan file format

Minimal plan:

```markdown
# Plan: Fix bugs

## Tasks
- [ ] Fix the off-by-one error in loop.go
- [ ] Add nil check in handler.go
```

With validation commands (run after each task):

```markdown
# Plan: Add authentication

## Validation Commands
- `go test ./...`
- `make lint`

## Tasks
- [ ] Add JWT middleware
- [ ] Add login endpoint
- [ ] Add tests for auth flow
```

## Source detection

`programmator start <thing>` auto-detects the source type:
- File paths (e.g. `./plan.md`) → plan file
- Everything else (e.g. `pro-1a2b`) → ticket ID (requires `ticket` CLI)

## Commands

```bash
programmator start ./plan.md              # execute a plan
programmator start ./plan.md --auto-commit # with git workflow (branch + commits)
programmator start pro-1a2b               # execute a ticket
programmator review                       # review current branch vs main
programmator review --base develop        # review against a different base
programmator run "explain this codebase"  # one-off prompt to the coding agent
programmator config show                  # show resolved config
```
