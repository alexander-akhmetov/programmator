# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prerequisites

Requires Go 1.26.0+ (see `go.mod`). Dependencies are vendored (`vendor/`).

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
go run ./cmd/programmator config show             # show resolved config

# Lint (matches CI: golangci-lint + govulncheck + deadcode + go mod tidy)
make lint

# Auto-fix formatting
make fmt

# E2E test prep
make e2e-prep                     # Create toy project for plan-based run
make e2e-review                   # Create toy project for review mode
```

## Architecture

Programmator is an autonomous coding agent orchestrator driven by tickets or plan files. It reads a source (ticket or plan file), identifies the current phase, invokes the configured executor (Claude Code or pi coding agent) with a structured prompt, parses the response, and loops until all phases are complete or safety limits are reached.

### Core Loop Flow

```
main.go (entry) → Loop.Run() → [for each iteration]:
    1. Source.Get() → fetch ticket/plan, parse phases
    2. BuildPrompt() → create Claude prompt with source context
    3. llm.Invoker.Invoke() → call configured executor via internal/llm
    4. ParseResponse() → extract PROGRAMMATOR_STATUS block (YAML)
    5. Source.UpdatePhase() → update checkbox in ticket/plan file
    6. CheckSafety() → verify iteration/stagnation limits
```

### Key Components

- **internal/loop/loop.go**: Main orchestration. Manages iteration state, invokes Claude via `llm.Invoker`, handles streaming JSON output. Supports process memory monitoring and auto-commit after phases.
- **internal/llm/**: Executor invocation layer. Defines `Invoker` interface with `ClaudeInvoker` and `PiInvoker` implementations, streaming JSON parsers, environment filtering, and hook support.
- **internal/domain/**: Core model types (`WorkItem`, `Phase`).
- **internal/protocol/**: Cross-package constants — status values (`CONTINUE`, `DONE`, `BLOCKED`, `REVIEW_PASS`, `REVIEW_FAIL`) and source type identifiers.
- **internal/source/**: Abstraction layer for work sources. `Source` interface with `TicketSource` and `PlanSource` implementations.
- **internal/source/detect.go**: Auto-detects source type from CLI argument (file path → plan, otherwise → ticket).
- **internal/ticket/client.go**: Wrapper around external `ticket` CLI. Parses markdown tickets with checkbox phases (`- [ ]`/`- [x]`). Has mock implementation for testing.
- **internal/plan/plan.go**: Parses standalone markdown plan files with checkbox tasks and optional validation commands. Supports `MoveTo()` for completed plan lifecycle.
- **internal/prompt/builder.go**: Builds prompts using Go `text/template` with named variables. Loads templates from embedded defaults, global, or local override files.
- **internal/parser/parser.go**: Extracts and parses `PROGRAMMATOR_STATUS` YAML block from Claude output. Status values: CONTINUE, DONE, BLOCKED.
- **internal/review/**: Code review pipeline. Runs parallel review agents, collects structured issues, validates findings, and builds fix prompts.
- **internal/event/**: Typed event system for communication between loop, CLI, and other components.
- **internal/safety/safety.go**: Exit conditions: max iterations, stagnation (no file changes), repeated errors.
- **internal/cli/**: CLI with streaming stdout event log, sticky ANSI footer, markdown rendering via glamour, and all command definitions (start, run, review, plan, status, config).
- **internal/config/**: Unified YAML configuration with multi-level merge (embedded defaults → global → env vars → local → CLI flags). Includes prompt template loading with fallback chain.
- **internal/dirs/**: XDG Base Directory Specification paths. Central source of truth for `ConfigDir()`, `StateDir()`, `LogsDir()`.
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

Unified YAML config with multi-level merge: embedded defaults → `~/.config/programmator/config.yaml` → `.programmator/config.yaml` → CLI flags. Run `programmator config show` to see resolved values.

### Prompt Templates

Prompts use Go `text/template` syntax. Override by placing files in `~/.config/programmator/prompts/` (global) or `.programmator/prompts/` (local). Templates: `phased.md`, `phaseless.md`, `review_first.md`.

## Testing

Tests use `stretchr/testify` for assertions. The ticket package has a mock client (`client_mock.go`) for testing without the external CLI. CI runs `go test -race` and `golangci-lint`.
Tests should use test cases pattern as much as possible to reuse setup and verifying code.

## Releasing

Push a git tag to trigger GitHub Actions release via GoReleaser:
```bash
git tag v1.0.0
git push origin v1.0.0
```
Publishes binaries for linux/darwin (amd64/arm64).
