#!/bin/bash
# Prepares a toy Go project for testing programmator review mode.
# Creates a project with code review issues (poor style, missing docs, etc).
#
# Usage: ./scripts/prep-review-test.sh
# Then: programmator review

set -e

TEST_DIR="/tmp/programmator-review-test"

echo "==> Cleaning up previous test project..."
rm -rf "$TEST_DIR"

echo "==> Creating test project at $TEST_DIR..."
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Initialize Go module
go mod init example.com/reviewproject

# Create main.go with code review issues
cat > main.go << 'EOF'
package main

import "fmt"
import "strings"
import "os"

func main() {
	x := os.Args
	if len(x) > 1 {
		y := x[1]
		z := processInput(y)
		fmt.Println(z)
	}
}

func processInput(s string) string {
	var result string
	result = ""
	for i := 0; i < len(s); i++ {
		c := string(s[i])
		if c != " " {
			result = result + strings.ToUpper(c)
		}
	}
	return result
}

func unusedFunction() {
	// This function is never called
	fmt.Println("dead code")
}

type data struct {
	value    int
	Name     string
	internal bool
}

func (d *data) getValue() int {
	return d.value
}
EOF

# Create a second file with issues
cat > utils.go << 'EOF'
package main

func helper(a int, b int, c int) int {
	if a > 0 {
		if b > 0 {
			if c > 0 {
				return a + b + c
			} else {
				return a + b
			}
		} else {
			return a
		}
	} else {
		return 0
	}
}
EOF

# Initialize git repo with initial commit
git init -q
git add -A
git commit -q -m "Initial commit"

# Make changes that will be reviewed
cat >> main.go << 'EOF'

func newFeature(input string) (string, error) {
	if input == "" {
		return "", nil
	}
	// TODO: implement this properly
	return input, nil
}
EOF

cat >> utils.go << 'EOF'

func anotherHelper(x int) int {
	// Magic number
	return x * 42
}
EOF

git add -A

echo ""
echo "==> Review test project created at $TEST_DIR"
echo ""
echo "The project has staged changes with code review issues:"
echo "  - Poor variable naming (x, y, z)"
echo "  - Grouped imports should use import block"
echo "  - Inefficient string concatenation"
echo "  - Unused function (dead code)"
echo "  - Inconsistent struct field visibility"
echo "  - Deeply nested conditionals"
echo "  - Magic numbers"
echo "  - TODO comment in production code"
echo ""
echo "To test programmator review, run:"
echo "  cd $TEST_DIR"
echo "  programmator review"
echo ""
echo "Or with auto-fix:"
echo "  programmator review --fix"
