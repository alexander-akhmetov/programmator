# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prerequisites

For ticket-based workflow, install the external `ticket` CLI:
```bash
brew tap alexander-akhmetov/tools git@github.com:alexander-akhmetov/homebrew-tools.git
brew install alexander-akhmetov/tools/ticket
```

Plan files work without any external dependencies.

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
go run ./cmd/programmator status
go run ./cmd/programmator logs <ticket-id>

# Lint (CI uses golangci-lint)
golangci-lint run
gofmt -l .                        # Check formatting
go vet ./...                      # Static analysis
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

- **internal/loop/loop.go**: Main orchestration. Manages iteration state, invokes Claude via os/exec, handles streaming JSON output. Supports pause/resume and process memory monitoring.
- **internal/source/**: Abstraction layer for work sources. `Source` interface with `TicketSource` and `PlanSource` implementations.
- **internal/source/detect.go**: Auto-detects source type from CLI argument (file path → plan, otherwise → ticket).
- **internal/ticket/client.go**: Wrapper around external `ticket` CLI. Parses markdown tickets with checkbox phases (`- [ ]`/`- [x]`). Has mock implementation for testing.
- **internal/plan/plan.go**: Parses standalone markdown plan files with checkbox tasks and optional validation commands.
- **internal/prompt/builder.go**: Builds prompts using `PromptTemplate`. Instructs Claude to output `PROGRAMMATOR_STATUS` block.
- **internal/parser/parser.go**: Extracts and parses `PROGRAMMATOR_STATUS` YAML block from Claude output. Status values: CONTINUE, DONE, BLOCKED.
- **internal/safety/safety.go**: Exit conditions: max iterations, stagnation (no file changes), repeated errors.
- **internal/tui/tui.go**: Bubbletea-based TUI with status panel, markdown rendering via glamour, and real-time token usage display.

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

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROGRAMMATOR_MAX_ITERATIONS` | 50 | Loop limit |
| `PROGRAMMATOR_STAGNATION_LIMIT` | 3 | Exit after N iterations with no file changes |
| `PROGRAMMATOR_TIMEOUT` | 900 | Seconds per Claude invocation |
| `PROGRAMMATOR_CLAUDE_FLAGS` | `--dangerously-skip-permissions` | Flags passed to Claude |
| `TICKETS_DIR` | `~/.tickets` | Where ticket files live |
| `CLAUDE_CONFIG_DIR` | - | Custom Claude config directory |

## Testing

Tests use `stretchr/testify` for assertions. The ticket package has a mock client (`client_mock.go`) for testing without the external CLI.
