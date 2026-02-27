# Linter and Test Check

Detect and run appropriate linting tools and tests if they are present and configured for the project.

## What to Do

1. Check for project configuration files (`go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`, `Makefile`, etc.)
2. Run the project's configured linters (e.g., `make lint`, `golangci-lint run ./...`, `npm run lint`, `cargo clippy`)
3. Run the project's test suite (e.g., `go test ./...`, `npm test`, `cargo test`, `pytest`)
4. Report any linter errors, warnings, or test failures as issues with file, line number, and description
5. Use severity "critical" for test failures, "high" for linter errors, "medium" for warnings

## Important

- Only run tools that are available and configured
- If a tool is not available, skip it
- If all checks pass, report an empty issues list
- Report problems only - no positive observations
