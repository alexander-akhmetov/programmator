# Linter Check

Detect and run appropriate linting tools if they are present and configured for the project.

## What to Do

1. Check for project configuration files (`go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`, `Makefile`, etc.)
2. Run the project's configured linters (e.g., `make lint`, `golangci-lint run ./...`, `npm run lint`, `cargo clippy`)
3. Report any linter errors or warnings as issues with file, line number, and description
4. Use severity "high" for errors, "medium" for warnings

## Important

- Only run linters that are available and configured
- If a linter is not available, skip it
- If all linters pass, report an empty issues list
- Report problems only - no positive observations
