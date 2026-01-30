package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_BasicPlan(t *testing.T) {
	content := `# Plan: Feature Implementation

## Validation Commands
- ` + "`go test ./...`" + `
- ` + "`golangci-lint run`" + `

## Tasks
- [ ] Task 1: Investigate the codebase
- [ ] Task 2: Implement the feature
- [x] Task 3: Already done
`

	plan, err := Parse("/path/to/plan.md", content)
	require.NoError(t, err)

	assert.Equal(t, "/path/to/plan.md", plan.FilePath)
	assert.Equal(t, "Feature Implementation", plan.Title)
	assert.Equal(t, []string{"go test ./...", "golangci-lint run"}, plan.ValidationCommands)
	assert.Len(t, plan.Tasks, 3)

	assert.Equal(t, "Task 1: Investigate the codebase", plan.Tasks[0].Name)
	assert.False(t, plan.Tasks[0].Completed)

	assert.Equal(t, "Task 2: Implement the feature", plan.Tasks[1].Name)
	assert.False(t, plan.Tasks[1].Completed)

	assert.Equal(t, "Task 3: Already done", plan.Tasks[2].Name)
	assert.True(t, plan.Tasks[2].Completed)
}

func TestParse_TitleWithoutPlanPrefix(t *testing.T) {
	content := `# My Feature

- [ ] Do something
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	assert.Equal(t, "My Feature", plan.Title)
}

func TestParse_TitleWithPlanPrefix(t *testing.T) {
	content := `# Plan: My Feature

- [ ] Do something
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	assert.Equal(t, "My Feature", plan.Title)
}

func TestParse_NoValidationCommands(t *testing.T) {
	content := `# Simple Plan

- [ ] Task 1
- [ ] Task 2
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	assert.Empty(t, plan.ValidationCommands)
	assert.Len(t, plan.Tasks, 2)
}

func TestParse_ValidationCommandsInSection(t *testing.T) {
	content := `# Plan

## Validation Commands
- ` + "`go test ./...`" + `
- ` + "`go vet ./...`" + `

## Tasks
- [ ] Do work
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	assert.Equal(t, []string{"go test ./...", "go vet ./..."}, plan.ValidationCommands)
}

func TestParse_ValidationSection(t *testing.T) {
	// Test with just "## Validation" instead of "## Validation Commands"
	content := `# Plan

## Validation
- ` + "`npm test`" + `

## Implementation
- [ ] Task 1
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	assert.Equal(t, []string{"npm test"}, plan.ValidationCommands)
}

func TestParse_CheckboxVariants(t *testing.T) {
	content := `# Plan

- [ ] Unchecked
- [x] Checked lowercase
- [X] Checked uppercase
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 3)

	assert.False(t, plan.Tasks[0].Completed)
	assert.True(t, plan.Tasks[1].Completed)
	assert.True(t, plan.Tasks[2].Completed)
}

func TestCurrentTask(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []Task
		expected *Task
	}{
		{
			name:     "no tasks",
			tasks:    []Task{},
			expected: nil,
		},
		{
			name: "all completed",
			tasks: []Task{
				{Name: "Task 1", Completed: true},
				{Name: "Task 2", Completed: true},
			},
			expected: nil,
		},
		{
			name: "first incomplete",
			tasks: []Task{
				{Name: "Task 1", Completed: false},
				{Name: "Task 2", Completed: false},
			},
			expected: &Task{Name: "Task 1", Completed: false},
		},
		{
			name: "second incomplete",
			tasks: []Task{
				{Name: "Task 1", Completed: true},
				{Name: "Task 2", Completed: false},
			},
			expected: &Task{Name: "Task 2", Completed: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &Plan{Tasks: tt.tasks}
			got := plan.CurrentTask()
			if tt.expected == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.expected.Name, got.Name)
			}
		})
	}
}

func TestAllTasksComplete(t *testing.T) {
	tests := []struct {
		name     string
		tasks    []Task
		expected bool
	}{
		{
			name:     "no tasks",
			tasks:    []Task{},
			expected: false,
		},
		{
			name: "all completed",
			tasks: []Task{
				{Name: "Task 1", Completed: true},
				{Name: "Task 2", Completed: true},
			},
			expected: true,
		},
		{
			name: "one incomplete",
			tasks: []Task{
				{Name: "Task 1", Completed: true},
				{Name: "Task 2", Completed: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &Plan{Tasks: tt.tasks}
			assert.Equal(t, tt.expected, plan.AllTasksComplete())
		})
	}
}

func TestMarkTaskComplete(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{Name: "Task 1: Investigation", Completed: false},
			{Name: "Task 2: Implementation", Completed: false},
		},
	}

	// Mark exact match
	err := plan.MarkTaskComplete("Task 1: Investigation")
	require.NoError(t, err)
	assert.True(t, plan.Tasks[0].Completed)

	// Mark partial match (normalized)
	err = plan.MarkTaskComplete("implementation")
	require.NoError(t, err)
	assert.True(t, plan.Tasks[1].Completed)
}

func TestMarkTaskComplete_ReverseContains(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{Name: "Task 1: Setup", Completed: false},
			{Name: "Task 2: Build", Completed: false},
		},
	}

	// Claude returns a longer name that contains the task name
	err := plan.MarkTaskComplete("Setup the project environment and dependencies")
	require.NoError(t, err)
	assert.True(t, plan.Tasks[0].Completed)
	assert.False(t, plan.Tasks[1].Completed)

	// Also works with prefix-stripped matching
	err = plan.MarkTaskComplete("Task 2: Build the application and run integration tests")
	require.NoError(t, err)
	assert.True(t, plan.Tasks[1].Completed)
}

func TestMarkTaskComplete_AlreadyCompleted(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{Name: "Task 1", Completed: true},
		},
	}

	err := plan.MarkTaskComplete("Task 1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found or already completed")
}

func TestMarkTaskComplete_NotFound(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{Name: "Task 1", Completed: false},
		},
	}

	err := plan.MarkTaskComplete("Task 99")
	assert.Error(t, err)
}

func TestMarkTaskComplete_WithPrefixes(t *testing.T) {
	plan := &Plan{
		Tasks: []Task{
			{Name: "Phase 1: Setup", Completed: false},
			{Name: "Step 2: Build", Completed: false},
		},
	}

	// Match with prefix stripping
	err := plan.MarkTaskComplete("setup")
	require.NoError(t, err)
	assert.True(t, plan.Tasks[0].Completed)

	err = plan.MarkTaskComplete("build")
	require.NoError(t, err)
	assert.True(t, plan.Tasks[1].Completed)
}

func TestPlanID(t *testing.T) {
	tests := []struct {
		filePath string
		expected string
	}{
		{"/path/to/feature.md", "feature"},
		{"/path/to/my-plan.md", "my-plan"},
		{"plan.md", "plan"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			plan := &Plan{FilePath: tt.filePath}
			assert.Equal(t, tt.expected, plan.ID())
		})
	}
}

func TestParseFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	content := `# Plan: Test Feature

## Validation Commands
- ` + "`go test ./...`" + `

## Tasks
- [ ] Implement feature
- [ ] Add tests
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	plan, err := ParseFile(planPath)
	require.NoError(t, err)

	assert.Equal(t, "Test Feature", plan.Title)
	assert.Len(t, plan.Tasks, 2)
	assert.Equal(t, []string{"go test ./..."}, plan.ValidationCommands)
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/plan.md")
	assert.Error(t, err)
}

func TestSaveFile(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "test-plan.md")

	content := `# Plan: Test

- [ ] Task 1
- [ ] Task 2
- [ ] Task 3
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	// Parse and modify
	plan, err := ParseFile(planPath)
	require.NoError(t, err)

	plan.Tasks[0].Completed = true
	plan.Tasks[2].Completed = true

	// Save
	err = plan.SaveFile()
	require.NoError(t, err)

	// Verify
	savedContent, err := os.ReadFile(planPath)
	require.NoError(t, err)

	assert.Contains(t, string(savedContent), "- [x] Task 1")
	assert.Contains(t, string(savedContent), "- [ ] Task 2")
	assert.Contains(t, string(savedContent), "- [x] Task 3")
}

func TestSaveFile_NoPath(t *testing.T) {
	plan := &Plan{Tasks: []Task{{Name: "Task", Completed: true}}}
	err := plan.SaveFile()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no file path")
}

func TestParse_NestedTasks(t *testing.T) {
	// Tasks under different sections should all be parsed
	content := `# Plan

### Section A
- [ ] Task A1
- [ ] Task A2

### Section B
- [x] Task B1
- [ ] Task B2
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)
	assert.Len(t, plan.Tasks, 4)

	assert.Equal(t, "Task A1", plan.Tasks[0].Name)
	assert.Equal(t, "Task A2", plan.Tasks[1].Name)
	assert.Equal(t, "Task B1", plan.Tasks[2].Name)
	assert.Equal(t, "Task B2", plan.Tasks[3].Name)

	assert.True(t, plan.Tasks[2].Completed)
}

func TestParse_RealWorldFormat(t *testing.T) {
	// Test the format from the ticket specification
	content := `# Plan: Feature Name

## Validation Commands
- ` + "`go test ./...`" + `
- ` + "`golangci-lint run`" + `

### Task 1: First task
- [ ] Do something
- [ ] Add tests

### Task 2: Second task
- [ ] Another thing
`
	plan, err := Parse("test.md", content)
	require.NoError(t, err)

	assert.Equal(t, "Feature Name", plan.Title)
	assert.Equal(t, []string{"go test ./...", "golangci-lint run"}, plan.ValidationCommands)
	assert.Len(t, plan.Tasks, 3)
}

func TestMoveTo(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	completedDir := filepath.Join(tmpDir, "plans", "completed")
	require.NoError(t, os.MkdirAll(plansDir, 0755))

	// Create a plan file
	planPath := filepath.Join(plansDir, "test-plan.md")
	content := `# Plan: Test

- [ ] Task 1
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	// Parse the plan
	plan, err := ParseFile(planPath)
	require.NoError(t, err)

	// Move to completed directory
	newPath, err := plan.MoveTo(completedDir)
	require.NoError(t, err)

	// Verify the new path
	expectedPath := filepath.Join(completedDir, "test-plan.md")
	assert.Equal(t, expectedPath, newPath)
	assert.Equal(t, expectedPath, plan.FilePath)

	// Verify the file exists at new location
	_, err = os.Stat(newPath)
	assert.NoError(t, err)

	// Verify the file doesn't exist at old location
	_, err = os.Stat(planPath)
	assert.True(t, os.IsNotExist(err))
}

func TestMoveTo_DestinationExists(t *testing.T) {
	tmpDir := t.TempDir()
	plansDir := filepath.Join(tmpDir, "plans")
	completedDir := filepath.Join(tmpDir, "plans", "completed")
	require.NoError(t, os.MkdirAll(plansDir, 0755))
	require.NoError(t, os.MkdirAll(completedDir, 0755))

	// Create source file
	planPath := filepath.Join(plansDir, "test-plan.md")
	require.NoError(t, os.WriteFile(planPath, []byte("# Plan\n- [ ] Task"), 0644))

	// Create destination file (conflict)
	destPath := filepath.Join(completedDir, "test-plan.md")
	require.NoError(t, os.WriteFile(destPath, []byte("# Old Plan"), 0644))

	// Parse the plan
	plan, err := ParseFile(planPath)
	require.NoError(t, err)

	// Move should fail
	_, err = plan.MoveTo(completedDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestMoveTo_NoFilePath(t *testing.T) {
	plan := &Plan{}
	_, err := plan.MoveTo("/some/dir")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no file path")
}
