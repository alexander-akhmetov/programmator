package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "git-repo-test-*")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(dir)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Create initial commit (git requires at least one commit for most operations)
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("# Test\n"), 0644))

	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	return dir, cleanup
}

func TestRepo_BranchExists(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Main/master branch should exist
	exists, err := repo.BranchExists("main")
	require.NoError(t, err)
	if !exists {
		exists, err = repo.BranchExists("master")
		require.NoError(t, err)
	}
	assert.True(t, exists)

	// Non-existent branch should not exist
	exists, err = repo.BranchExists("nonexistent-branch-xyz")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRepo_CreateBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Create a new branch
	err := repo.CreateBranch("feature/test")
	require.NoError(t, err)

	// Should be on the new branch
	current, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "feature/test", current)

	// Branch should exist
	exists, err := repo.BranchExists("feature/test")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRepo_CreateBranch_ExistingBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Create a branch
	err := repo.CreateBranch("feature/test")
	require.NoError(t, err)

	// Switch back to main/master
	mainBranch := "main"
	if exists, _ := repo.BranchExists("main"); !exists {
		mainBranch = "master"
	}
	err = repo.CheckoutBranch(mainBranch)
	require.NoError(t, err)

	// Create the same branch again - should just checkout
	err = repo.CreateBranch("feature/test")
	require.NoError(t, err)

	// Should be on the branch
	current, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "feature/test", current)
}

func TestRepo_AddAndCommit(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Create a file
	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Add and commit
	err := repo.AddAndCommit([]string{"test.txt"}, "Add test file")
	require.NoError(t, err)

	// No uncommitted changes should remain
	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges)
}

func TestRepo_AddAndCommit_NoChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Add and commit with no files - should not error
	err := repo.AddAndCommit([]string{}, "Empty commit")
	require.NoError(t, err)
}

func TestRepo_MoveFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Create and commit a file
	testFile := filepath.Join(dir, "source.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))
	err := repo.AddAndCommit([]string{"source.txt"}, "Add source file")
	require.NoError(t, err)

	// Create destination directory
	destDir := filepath.Join(dir, "completed")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	// Move the file
	err = repo.MoveFile("source.txt", "completed/source.txt")
	require.NoError(t, err)

	// Original should not exist
	_, err = os.Stat(testFile)
	assert.True(t, os.IsNotExist(err))

	// Destination should exist
	_, err = os.Stat(filepath.Join(dir, "completed", "source.txt"))
	assert.NoError(t, err)
}

func TestRepo_HasUncommittedChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo := NewRepo(dir)

	// Initially no uncommitted changes
	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges)

	// Create a new file
	testFile := filepath.Join(dir, "new.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("new content"), 0644))

	// Now should have uncommitted changes
	hasChanges, err = repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.True(t, hasChanges)
}

func TestBranchNameFromSource(t *testing.T) {
	tests := []struct {
		name     string
		sourceID string
		isPlan   bool
		want     string
	}{
		{
			name:     "plan file with path",
			sourceID: "./plans/add-auth.md",
			isPlan:   true,
			want:     "programmator/add-auth",
		},
		{
			name:     "plan file simple",
			sourceID: "feature.md",
			isPlan:   true,
			want:     "programmator/feature",
		},
		{
			name:     "ticket id",
			sourceID: "pro-1234",
			isPlan:   false,
			want:     "programmator/pro-1234",
		},
		{
			name:     "ticket with special chars",
			sourceID: "issue:123",
			isPlan:   false,
			want:     "programmator/issue-123",
		},
		{
			name:     "plan with spaces",
			sourceID: "my plan file.md",
			isPlan:   true,
			want:     "programmator/my-plan-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BranchNameFromSource(tt.sourceID, tt.isPlan)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with spaces", "with-spaces"},
		{"with:colons", "with-colons"},
		{"with?question", "with-question"},
		{"with*asterisk", "with-asterisk"},
		{"with[brackets]", "with-brackets"},
		{"multiple---dashes", "multiple-dashes"},
		{".leading-dot", "leading-dot"},
		{"trailing-dot.", "trailing-dot"},
		{"-leading-dash", "leading-dash"},
		{"trailing-dash-", "trailing-dash"},
		{"", "work"},
		{"---", "work"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBranchName(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
