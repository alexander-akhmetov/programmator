package source

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestPlanSource_Get(t *testing.T) {
	// Create temp plan file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := `# Plan: Test Feature

## Validation Commands
- ` + "`go test ./...`" + `

## Tasks
- [ ] Task 1: Investigation
- [x] Task 2: Already done
- [ ] Task 3: Implementation
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)
	item, err := source.Get(planPath)
	require.NoError(t, err)

	assert.Equal(t, "test-plan", item.ID)
	assert.Equal(t, "Test Feature", item.Title)
	assert.Equal(t, protocol.WorkItemOpen, item.Status)
	assert.Len(t, item.Phases, 3)
	assert.Equal(t, []string{"go test ./..."}, item.ValidationCommands)

	assert.Equal(t, "Task 1: Investigation", item.Phases[0].Name)
	assert.False(t, item.Phases[0].Completed)

	assert.Equal(t, "Task 2: Already done", item.Phases[1].Name)
	assert.True(t, item.Phases[1].Completed)

	assert.Equal(t, "Task 3: Implementation", item.Phases[2].Name)
	assert.False(t, item.Phases[2].Completed)
}

func TestPlanSource_UpdatePhase(t *testing.T) {
	// Create temp plan file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := `# Plan: Test

- [ ] Task 1
- [ ] Task 2
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)

	// Mark first task complete
	err = source.UpdatePhase(planPath, "Task 1")
	require.NoError(t, err)

	// Verify by re-reading
	item, err := source.Get(planPath)
	require.NoError(t, err)

	assert.True(t, item.Phases[0].Completed)
	assert.False(t, item.Phases[1].Completed)

	// Verify file content
	savedContent, err := os.ReadFile(planPath)
	require.NoError(t, err)
	assert.Contains(t, string(savedContent), "- [x] Task 1")
	assert.Contains(t, string(savedContent), "- [ ] Task 2")
}

func TestPlanSource_AddNote_NoOp(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := "# Plan\n- [ ] Task\n"
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)

	// AddNote should be a no-op (not error)
	err = source.AddNote(planPath, "some note")
	assert.NoError(t, err)

	// Content should be unchanged
	savedContent, err := os.ReadFile(planPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(savedContent))
}

func TestPlanSource_SetStatus_NoOp(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := "# Plan\n- [ ] Task\n"
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)

	// SetStatus should be a no-op (not error)
	err = source.SetStatus(planPath, protocol.WorkItemClosed)
	assert.NoError(t, err)
}

func TestPlanSource_Type(t *testing.T) {
	source := NewPlanSource("/any/path")
	assert.Equal(t, TypePlan, source.Type())
}

func TestPlanSource_Get_NotFound(t *testing.T) {
	source := NewPlanSource("/nonexistent/path/plan.md")
	_, err := source.Get("/nonexistent/path/plan.md")
	assert.Error(t, err)
}

func TestPlanSource_UpdatePhase_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := "# Plan\n- [ ] Task 1\n"
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)

	err = source.UpdatePhase(planPath, "Nonexistent Task")
	assert.Error(t, err)
}

func TestPlanSource_UpdatePhase_MultiplePhases(t *testing.T) {
	// Test updating multiple phases in sequence
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := `# Plan: Multi-phase Test

## Tasks
- [ ] Phase 1: Investigation
- [ ] Phase 2: Implementation
- [ ] Phase 3: Testing
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)

	// Mark phases complete in order
	err = source.UpdatePhase(planPath, "Phase 1: Investigation")
	require.NoError(t, err)

	err = source.UpdatePhase(planPath, "Phase 2: Implementation")
	require.NoError(t, err)

	// Verify file content
	savedContent, err := os.ReadFile(planPath)
	require.NoError(t, err)

	assert.Contains(t, string(savedContent), "- [x] Phase 1: Investigation")
	assert.Contains(t, string(savedContent), "- [x] Phase 2: Implementation")
	assert.Contains(t, string(savedContent), "- [ ] Phase 3: Testing")

	// Verify via Get
	item, err := source.Get(planPath)
	require.NoError(t, err)

	assert.True(t, item.Phases[0].Completed)
	assert.True(t, item.Phases[1].Completed)
	assert.False(t, item.Phases[2].Completed)
}

func TestPlanSource_UpdatePhase_PreservesFormatting(t *testing.T) {
	// Ensure that updating a phase preserves other content in the file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")
	content := `# Plan: Formatting Test

## Description
This is a test plan with some content.

## Validation Commands
- ` + "`go test ./...`" + `
- ` + "`golangci-lint run`" + `

## Tasks
- [ ] Task 1: First task

### Notes
Some important notes here.
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	source := NewPlanSource(planPath)

	err = source.UpdatePhase(planPath, "Task 1: First task")
	require.NoError(t, err)

	savedContent, err := os.ReadFile(planPath)
	require.NoError(t, err)

	// Verify checkbox updated
	assert.Contains(t, string(savedContent), "- [x] Task 1: First task")

	// Verify other content preserved
	assert.Contains(t, string(savedContent), "# Plan: Formatting Test")
	assert.Contains(t, string(savedContent), "## Description")
	assert.Contains(t, string(savedContent), "This is a test plan with some content.")
	assert.Contains(t, string(savedContent), "## Validation Commands")
	assert.Contains(t, string(savedContent), "`go test ./...`")
	assert.Contains(t, string(savedContent), "### Notes")
	assert.Contains(t, string(savedContent), "Some important notes here.")
}
