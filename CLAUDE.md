# CLAUDE.md

## Build & Test

```bash
go build ./...              # Build
go test ./...               # All tests
go test ./internal/parser   # Single package
go test -race ./...         # Race detector (CI uses this)
make lint                   # golangci-lint + govulncheck + deadcode + go mod tidy
make fmt                    # Auto-fix formatting
```

## Code Conventions

- Tests use `stretchr/testify`. Prefer test cases pattern to reuse setup and verification.
- Dependencies vendored in `vendor/`.
- CI runs `go test -race` and `golangci-lint`.

## Package Layout

Entry: `cmd/programmator/main.go` → `internal/cli/` → `internal/loop/loop.go`

- `internal/loop/` — main orchestration loop
- `internal/llm/` — executor invocation (ClaudeInvoker, PiInvoker), streaming JSON parsers
- `internal/source/` — work source abstraction (PlanSource, TicketSource), auto-detection in `detect.go`
- `internal/review/` — multi-agent review pipeline, parallel execution, validators
- `internal/parser/` — PROGRAMMATOR_STATUS YAML extraction from executor output
- `internal/prompt/` — Go `text/template` prompt builder with embedded/global/local fallback
- `internal/config/` — YAML config with multi-level merge; defaults in `internal/config/defaults/`
- `internal/domain/` — core types (WorkItem, Phase)
- `internal/protocol/` — cross-package constants (status values, source types)
- `internal/event/` — typed event system for loop ↔ CLI communication
- `internal/safety/` — exit conditions (max iterations, stagnation, error repetition)
- `internal/git/` — git operations wrapper for auto-commit workflow
- `internal/dirs/` — XDG paths (ConfigDir, StateDir, LogsDir)
- `internal/ticket/` — external `ticket` CLI wrapper; mock in `client_mock.go`
- `internal/plan/` — plan file parser with checkbox tasks and validation commands
