#!/bin/bash
# Prepares a toy Go project for testing programmator with a plan file.
# Creates a project with intentional bugs that the plan instructs Claude to fix.
#
# Usage: ./scripts/prep-toy-test.sh
# Then: programmator start /tmp/programmator-test/plans/fix-issues.md

set -e

TEST_DIR="/tmp/programmator-test"

echo "==> Cleaning up previous test project..."
rm -rf "$TEST_DIR"

echo "==> Creating test project at $TEST_DIR..."
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Initialize Go module
go mod init example.com/testproject

# Create main.go with intentional bugs
cat > main.go << 'EOF'
package main

import (
	"fmt"
)

func main() {
	// Bug 1: Function returns wrong value
	result := add(2, 3)
	fmt.Printf("2 + 3 = %d\n", result)

	// Bug 2: Off-by-one error
	items := []string{"a", "b", "c"}
	for i := 0; i <= len(items); i++ {
		fmt.Println(items[i])
	}

	// Bug 3: Nil pointer dereference
	var user *User
	fmt.Println(user.Name)
}

// add should return a + b, but it returns a - b
func add(a, b int) int {
	return a - b
}

type User struct {
	Name string
}
EOF

# Create test file
cat > main_test.go << 'EOF'
package main

import "testing"

func TestAdd(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{2, 3, 5},
		{0, 0, 0},
		{-1, 1, 0},
		{10, -5, 5},
	}

	for _, tt := range tests {
		got := add(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
EOF

# Create plans directory with a fix-issues plan
mkdir -p plans
cat > plans/fix-issues.md << 'EOF'
# Plan: Fix Bugs in Test Project

Fix the three intentional bugs in the test project.

## Validation Commands
- `go build ./...`
- `go test ./...`

## Tasks
- [ ] Fix the add() function to return a + b instead of a - b
- [ ] Fix the off-by-one error in the items loop (use < instead of <=)
- [ ] Fix the nil pointer dereference by initializing the user variable
EOF

# Initialize git repo
git init -q
git add -A
git commit -q -m "Initial commit with intentional bugs"

echo ""
echo "==> Test project created at $TEST_DIR"
echo ""
echo "The project has 3 intentional bugs:"
echo "  1. add() returns a - b instead of a + b"
echo "  2. Off-by-one error in loop (i <= len instead of i < len)"
echo "  3. Nil pointer dereference (uninitialized User pointer)"
echo ""
echo "To test programmator, run:"
echo "  cd $TEST_DIR"
echo "  programmator start ./plans/fix-issues.md"
echo ""
echo "Or with auto-commit:"
echo "  programmator start ./plans/fix-issues.md --auto-commit"
