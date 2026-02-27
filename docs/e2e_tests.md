# E2E Tests

Programmator relies on manual end-to-end tests for integration checks. Each test creates a small, isolated Go project in `/tmp/` with known issues, then you run programmator against it to validate behavior.

These are intentionally **not** automated CI tests. They require a working Claude Code installation and are meant for manual verification during development.

## Available Tests

### Plan-based run (`make e2e-prep`)

Creates `/tmp/programmator-test/` — a Go project with three intentional bugs and a plan file that asks Claude to fix them.

**What it includes:**
- `add()` returns `a - b` instead of `a + b`
- Off-by-one error in a loop (`<=` instead of `<`)
- Nil pointer dereference (uninitialized pointer)

**Run it:**
```bash
make e2e-prep
cd /tmp/programmator-test
programmator start ./plans/fix-issues.md
# or with auto-commit:
programmator start ./plans/fix-issues.md --auto-commit
```

**What to verify:** All three bugs get fixed, tests pass, and plan checkboxes get marked complete.

### Review mode (`make e2e-review`)

Creates `/tmp/programmator-review-test/` — a Go project with code quality issues and a feature branch containing committed changes for review.

**What it includes:**
- Poor variable naming (`x`, `y`, `z`)
- Ungrouped imports
- Inefficient string concatenation
- Dead code (unused function)
- Deeply nested conditionals
- Magic numbers

**Run it:**
```bash
make e2e-review
cd /tmp/programmator-review-test
programmator review
```

**What to verify:** Review identifies the issues and performs fix iterations. Check `git log --oneline` for fix commits and verify the code quality improves.

## Creating a New E2E Test

1. Create a new script in `scripts/`:

```bash
#!/bin/bash
# Prepares a small project for testing <feature>.
#
# Usage: ./scripts/prep-<name>-test.sh
# Then: programmator <command>

set -e

TEST_DIR="/tmp/programmator-<name>-test"

echo "==> Cleaning up previous test project..."
rm -rf "$TEST_DIR"

echo "==> Creating test project at $TEST_DIR..."
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Initialize Go module
go mod init example.com/testproject

# Create source files with known issues or starting state
cat > main.go << 'EOF'
package main
// ...
EOF

# Initialize git repo (required for most programmator features)
git init -q
git add -A
git commit -q -m "Initial commit"

echo ""
echo "==> Test project created at $TEST_DIR"
echo "To test, run:"
echo "  cd $TEST_DIR"
echo "  programmator <command>"
```

2. Make it executable:

```bash
chmod +x scripts/prep-<name>-test.sh
```

3. Add a Makefile target:

```makefile
e2e-<name>:
	@./scripts/prep-<name>-test.sh
```

4. Add the target to the `.PHONY` list in the Makefile.

### Guidelines

- Always clean up previous runs (`rm -rf "$TEST_DIR"`) so the script is idempotent.
- Use `/tmp/programmator-*-test/` naming to keep test projects grouped.
- Initialize a git repo — most programmator features depend on it.
- Include clear output messages explaining what was created and how to run it.
- Add validation commands (`go test`, `go build`) when the test involves code changes.
- Keep the project small — just enough to exercise the feature under test.
