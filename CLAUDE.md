# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prerequisites

Requires Go 1.25.6+ (see `go.mod`).

Plan files work without any external dependencies. Ticket-based workflow requires the external `ticket` CLI.

## Build and Test Commands

```bash
go build ./...                    # Build all packages
go test ./...                     # Run all tests
go test ./internal/parser -v      # Run single package tests
go test -race ./...               # Run tests with race detector (CI uses this)

# Install the CLI
go install ./cmd/programmator

# Run without installing
go run ./cmd/programmator start <ticket-id>      # ticket
go run ./cmd/programmator start ./plan.md        # plan file
go run ./cmd/programmator start ./plan.md --auto-commit  # with auto git workflow
go run ./cmd/programmator status
go run ./cmd/programmator logs <ticket-id>
go run ./cmd/programmator logs --follow           # tail active log
go run ./cmd/programmator plan create "description"  # interactive plan creation
go run ./cmd/programmator config show             # show resolved config

# Lint (CI uses golangci-lint)
golangci-lint run
gofmt -l .                        # Check formatting
go vet ./...                      # Static analysis

# E2E test prep
make e2e-prep                     # Create toy project for plan-based run
make e2e-review                   # Create toy project for review mode
make e2e-plan                     # Create toy project for plan creation
```

## Architecture

Programmator is an autonomous Claude Code orchestrator driven by tickets or plan files. It reads a source (ticket or plan file), identifies the current phase, invokes Claude Code with a structured prompt, parses the response, and loops until all phases are complete or safety limits are reached.

### Core Loop Flow

```
main.go (entry) → Loop.Run() → [for each iteration]:
    1. Source.Get() → fetch ticket/plan, parse phases
    2. BuildPrompt() → create Claude prompt with source context
    3. invokeClaudeCode() → exec call to `claude --print`
    4. ParseResponse() → extract PROGRAMMATOR_STATUS block (YAML)
    5. Source.MarkPhaseComplete() → update checkbox in ticket/plan file
    6. CheckSafety() → verify iteration/stagnation limits
```

### Key Components

- **internal/loop/loop.go**: Main orchestration. Manages iteration state, invokes Claude via os/exec, handles streaming JSON output. Supports pause/resume, process memory monitoring, auto-commit after phases, and progress logging.
- **internal/source/**: Abstraction layer for work sources. `Source` interface with `TicketSource` and `PlanSource` implementations.
- **internal/source/detect.go**: Auto-detects source type from CLI argument (file path → plan, otherwise → ticket).
- **internal/ticket/client.go**: Wrapper around external `ticket` CLI. Parses markdown tickets with checkbox phases (`- [ ]`/`- [x]`). Has mock implementation for testing.
- **internal/plan/plan.go**: Parses standalone markdown plan files with checkbox tasks and optional validation commands. Supports `MoveTo()` for completed plan lifecycle.
- **internal/prompt/builder.go**: Builds prompts using Go `text/template` with named variables. Loads templates from embedded defaults, global, or local override files.
- **internal/parser/parser.go**: Extracts and parses `PROGRAMMATOR_STATUS` YAML block from Claude output. Status values: CONTINUE, DONE, BLOCKED. Also parses `PROGRAMMATOR_QUESTION` and `PROGRAMMATOR_PLAN_READY` signals for interactive plan creation.
- **internal/safety/safety.go**: Exit conditions: max iterations, stagnation (no file changes), repeated errors.
- **internal/tui/tui.go**: Bubbletea-based TUI with status panel, markdown rendering via glamour, and real-time token usage display.
- **internal/config/**: Unified YAML configuration with multi-level merge (embedded defaults → global → env vars → local → CLI flags). Includes prompt template loading with fallback chain.
- **internal/progress/**: Persistent run logging. `Logger` writes timestamped entries to `~/.programmator/logs/`. Includes file locking (`flock_unix.go`) for active session detection.
- **internal/input/**: User input collection for interactive plan creation. `Collector` interface with `TerminalCollector` (fzf with numbered fallback).
- **internal/git/repo.go**: Git operations wrapper (`Repo` struct) for branch creation, checkout, add, commit, and file moves. Used by auto-commit workflow.

### Status Protocol

Claude must output this block at the end of each response:
```yaml
PROGRAMMATOR_STATUS:
  phase_completed: "Phase name" or null
  status: CONTINUE | DONE | BLOCKED
  files_changed: [list of files]
  summary: "what was done"
  error: "blocking reason" (only if BLOCKED)
```

### Source Formats

**Tickets**: Markdown files with YAML frontmatter. Phases are checkboxes in a Design section:
```markdown
## Design
- [ ] Phase 1: Investigation
- [x] Phase 2: Implementation (completed)
```

**Plan files**: Standalone markdown with checkbox tasks and optional validation commands:
```markdown
# Plan: Feature Name

## Validation Commands
- `go test ./...`

## Tasks
- [ ] Task 1: Investigate
- [ ] Task 2: Implement
```

## Configuration

Unified YAML config with multi-level merge: embedded defaults → `~/.config/programmator/config.yaml` → env vars → `.programmator/config.yaml` → CLI flags. Run `programmator config show` to see resolved values.

### Environment Variables (Legacy)

| Variable | Default | Description |
|----------|---------|-------------|
| `PROGRAMMATOR_MAX_ITERATIONS` | 50 | Loop limit |
| `PROGRAMMATOR_STAGNATION_LIMIT` | 3 | Exit after N iterations with no file changes |
| `PROGRAMMATOR_TIMEOUT` | 900 | Seconds per Claude invocation |
| `PROGRAMMATOR_CLAUDE_FLAGS` | `""` | Flags passed to Claude |
| `TICKETS_DIR` | `~/.tickets` | Where ticket files live |
| `CLAUDE_CONFIG_DIR` | - | Custom Claude config directory |
| `PROGRAMMATOR_ANTHROPIC_API_KEY` | - | Anthropic API key forwarded to Claude (`ANTHROPIC_API_KEY` is filtered from inherited env) |

### Prompt Templates

Prompts use Go `text/template` syntax. Override by placing files in `~/.config/programmator/prompts/` (global) or `.programmator/prompts/` (local). Templates: `phased.md`, `phaseless.md`, `review_first.md`, `review_second.md`, `plan_create.md`.

## Testing

Tests use `stretchr/testify` for assertions. The ticket package has a mock client (`client_mock.go`) for testing without the external CLI. CI runs `go test -race` and `golangci-lint`.

## Releasing

Push a git tag to trigger GitHub Actions release via GoReleaser:
```bash
git tag v1.0.0
git push origin v1.0.0
```
Publishes binaries for linux/darwin (amd64/arm64).
