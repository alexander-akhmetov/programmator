package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLooksLikeFilePath(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// File paths
		{"/path/to/file.md", true},
		{"./plan.md", true},
		{"../docs/plan.md", true},
		{"docs/feature.md", true},
		{"plan.md", true},
		{"feature.MD", true},
		{".hidden", true},

		// Ticket IDs
		{"pro-1234", false},
		{"ticket-abc", false},
		{"FEAT-123", false},
		{"abc123", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := looksLikeFilePath(tt.input)
			assert.Equal(t, tt.expected, got, "looksLikeFilePath(%q)", tt.input)
		})
	}
}

func TestDetect_PlanFile(t *testing.T) {
	// Create temp plan file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "feature.md")
	content := `# Plan: Test Feature
- [ ] Task 1
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source, id := Detect(planPath, "")
	assert.IsType(t, &PlanSource{}, source)
	assert.Equal(t, planPath, id)
	assert.Equal(t, "plan", source.Type())
}

func TestDetect_TicketID(t *testing.T) {
	source, id := Detect("pro-1234", "")
	assert.IsType(t, &TicketSource{}, source)
	assert.Equal(t, "pro-1234", id)
	assert.Equal(t, "ticket", source.Type())
}

func TestDetect_RelativePath(t *testing.T) {
	// Test with path that looks like a file but doesn't exist
	source, id := Detect("./nonexistent/plan.md", "")
	assert.IsType(t, &PlanSource{}, source)
	assert.Equal(t, "./nonexistent/plan.md", id)
}

func TestDetect_ExistingFile(t *testing.T) {
	// Create temp file without .md extension
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan-file")
	err := os.WriteFile(planPath, []byte("# Test\n"), 0644)
	require.NoError(t, err)

	// Since it exists, should be treated as plan
	source, id := Detect(planPath, "")
	assert.IsType(t, &PlanSource{}, source)
	assert.NotEmpty(t, id)
}

func TestIsPlanPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"md extension", "feature.md", true},
		{"full path", "/path/to/plan.md", true},
		{"relative path", "./plan.md", true},
		{"ticket id", "pro-1234", false},
		{"simple name", "myticket", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPlanPath(tt.path)
			assert.Equal(t, tt.expected, got)
		})
	}
}
