# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

```bash
uv sync              # Install dependencies
uv run pytest -v     # Run all tests
uv run pytest tests/test_response_parser.py -v  # Run single test file

# Lint and format
uv run ruff format --check src tests   # Check formatting
uv run ruff check src tests            # Lint
uv run ty check src tests              # Type check

uv run ruff format src tests           # Auto-format
uv run ruff check --fix src tests      # Auto-fix lint issues
```

## Architecture

Programmator is a ticket-driven autonomous Claude Code orchestrator. It reads a ticket, identifies the current phase, invokes Claude Code with a structured prompt, parses the response, and loops until all phases are complete or safety limits are reached.

### Core Loop Flow

```
cli.py (entry) → Loop.run() → [for each iteration]:
    1. ticket_client.get() → fetch ticket, parse phases
    2. build_prompt() → create Claude prompt with ticket context
    3. _invoke_claude() → subprocess call to `claude --print`
    4. parse_response() → extract PROGRAMMATOR_STATUS block (YAML)
    5. ticket_client.update_phase() → mark phase complete
    6. check_safety() → verify iteration/stagnation limits
```

### Key Components

- **loop.py**: Main orchestration. Manages iteration state, invokes Claude via subprocess, handles streaming JSON output
- **ticket_client.py**: Wrapper around external `ticket` CLI. Parses markdown tickets with YAML frontmatter and checkbox phases (`- [ ]`/`- [x]`)
- **prompt_builder.py**: Builds prompts using `PROMPT_TEMPLATE`. Instructs Claude to output `PROGRAMMATOR_STATUS` block
- **response_parser.py**: Extracts and parses `PROGRAMMATOR_STATUS` YAML block from Claude output. Status values: CONTINUE, DONE, BLOCKED
- **safety.py**: Exit conditions: max iterations, stagnation (no file changes), repeated errors
- **tui.py**: Textual-based TUI with status panel and log viewer (always enabled)

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
