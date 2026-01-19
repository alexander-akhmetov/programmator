# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
go build ./...                    # Build all packages
go test ./...                     # Run all tests
go test ./internal/parser -v      # Run single package tests
go test -race ./...               # Run tests with race detector

# Install the CLI
go install ./cmd/programmator

# Run without installing
go run ./cmd/programmator start <ticket-id>
go run ./cmd/programmator status <ticket-id>
go run ./cmd/programmator logs <ticket-id>

# Lint and format
gofmt -l .                        # Check formatting
gofmt -w .                        # Auto-format
go vet ./...                      # Static analysis
```

## Architecture

Programmator is a ticket-driven autonomous Claude Code orchestrator. It reads a ticket, identifies the current phase, invokes Claude Code with a structured prompt, parses the response, and loops until all phases are complete or safety limits are reached.

### Core Loop Flow

```
main.go (entry) → Loop.Run() → [for each iteration]:
    1. TicketClient.Get() → fetch ticket, parse phases
    2. BuildPrompt() → create Claude prompt with ticket context
    3. invokeClaudeCode() → exec call to `claude --print`
    4. ParseResponse() → extract PROGRAMMATOR_STATUS block (YAML)
    5. TicketClient.UpdatePhase() → mark phase complete
    6. CheckSafety() → verify iteration/stagnation limits
```

### Directory Structure

```
cmd/
  programmator/
    main.go           # CLI entry point (cobra commands)
internal/
  cmd/
    root.go           # Root cobra command
    start.go          # Start command (runs the loop)
    status.go         # Status command (shows ticket state)
    logs.go           # Logs command (tails log file)
  loop/
    loop.go           # Main orchestration loop
  ticket/
    client.go         # Ticket CLI wrapper, parses markdown tickets
  prompt/
    builder.go        # Builds prompts with ticket context
  parser/
    parser.go         # Extracts PROGRAMMATOR_STATUS YAML block
  safety/
    safety.go         # Exit conditions (max iterations, stagnation)
  tui/
    tui.go            # Bubbletea TUI with status panel and logs
```

### Key Components

- **internal/loop/loop.go**: Main orchestration. Manages iteration state, invokes Claude via os/exec, handles streaming output
- **internal/ticket/client.go**: Wrapper around external `ticket` CLI. Parses markdown tickets with checkbox phases (`- [ ]`/`- [x]`)
- **internal/prompt/builder.go**: Builds prompts using `PromptTemplate`. Instructs Claude to output `PROGRAMMATOR_STATUS` block
- **internal/parser/parser.go**: Extracts and parses `PROGRAMMATOR_STATUS` YAML block from Claude output. Status values: CONTINUE, DONE, BLOCKED
- **internal/safety/safety.go**: Exit conditions: max iterations, stagnation (no file changes), repeated errors
- **internal/tui/tui.go**: Bubbletea-based TUI with status panel and log viewer

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

### Ticket Format

Tickets are markdown files with YAML frontmatter. Phases are checkboxes in a Design section:
```markdown
## Design
- [ ] Phase 1: Investigation
- [x] Phase 2: Implementation (completed)
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PROGRAMMATOR_MAX_ITERATIONS` | 50 | Loop limit |
| `PROGRAMMATOR_STAGNATION_LIMIT` | 3 | Exit after N iterations with no file changes |
| `PROGRAMMATOR_TIMEOUT` | 900 | Seconds per Claude invocation |
| `PROGRAMMATOR_CLAUDE_FLAGS` | `--dangerously-skip-permissions` | Flags passed to Claude |
| `TICKETS_DIR` | `~/.tickets` | Where ticket files live |

## Dependencies

- [cobra](https://github.com/spf13/cobra) - CLI framework
- [bubbletea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [lipgloss](https://github.com/charmbracelet/lipgloss) - TUI styling
- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) - YAML parsing
