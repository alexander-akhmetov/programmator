package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
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

	// Initialize git repo with go-git
	r, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	// Configure git user for commits
	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@test.com"
	err = r.SetConfig(cfg)
	require.NoError(t, err)

	// Create initial commit
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("# Test\n"), 0644))

	wt, err := r.Worktree()
	require.NoError(t, err)

	_, err = wt.Add("README.md")
	require.NoError(t, err)

	_, err = wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	return dir, cleanup
}

func TestRepo_BranchExists(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

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

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	// Create a new branch
	err = repo.CreateBranch("feature/test")
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

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	// Create a branch
	err = repo.CreateBranch("feature/test")
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

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	// Create a file
	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0644))

	// Add and commit
	err = repo.AddAndCommit([]string{"test.txt"}, "Add test file")
	require.NoError(t, err)

	// No uncommitted changes should remain
	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges)
}

func TestRepo_AddAndCommit_NoChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	// Record HEAD before attempt
	r, err := gogit.PlainOpen(dir)
	require.NoError(t, err)
	headBefore, err := r.Head()
	require.NoError(t, err)

	// Add and commit with no files - should not error
	err = repo.AddAndCommit([]string{}, "Empty commit")
	require.NoError(t, err)

	// Verify HEAD did not move (no commit was created)
	headAfter, err := r.Head()
	require.NoError(t, err)
	assert.Equal(t, headBefore.Hash(), headAfter.Hash(), "HEAD should not move when committing with no files")
}

func TestRepo_MoveFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	// Create and commit a file
	testFile := filepath.Join(dir, "source.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))
	err = repo.AddAndCommit([]string{"source.txt"}, "Add source file")
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

	repo, err := NewRepo(dir)
	require.NoError(t, err)

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

func TestNewRepo_NonGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := NewRepo(tmpDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open git repo")
}

func TestRepo_Add_PathTraversal(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	err = repo.Add("../../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal not allowed")

	err = repo.Add("/etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute path not allowed")
}

func TestRepo_MoveFile_PathTraversal(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	err = repo.MoveFile("../outside", "dest.txt")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal not allowed")

	err = repo.MoveFile("source.txt", "../outside")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path traversal not allowed")
}

func TestRepo_CheckoutBranch_NonExistent(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	err = repo.CheckoutBranch("nonexistent-branch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checkout branch")
}

func TestRepo_WorkDir(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	assert.Equal(t, dir, repo.WorkDir())
}

func TestRepo_CommitSignatureFromLocalConfig(t *testing.T) {
	// Create a repo with user.name/user.email in local config
	dir := t.TempDir()
	r, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Local User"
	cfg.User.Email = "local@test.com"
	require.NoError(t, r.SetConfig(cfg))

	// Create initial commit with explicit author (required by go-git)
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("# Test\n"), 0644))
	wt, err := r.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Setup",
			Email: "setup@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Open via NewRepo and create a file to commit
	repo, err := NewRepo(dir)
	require.NoError(t, err)

	testFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	err = repo.AddAndCommit([]string{"test.txt"}, "Test commit with local config")
	require.NoError(t, err)

	// Verify the commit used local config values
	head, err := r.Head()
	require.NoError(t, err)
	commit, err := r.CommitObject(head.Hash())
	require.NoError(t, err)

	assert.Equal(t, "Local User", commit.Author.Name)
	assert.Equal(t, "local@test.com", commit.Author.Email)
}

func TestRepo_ChangedFilesFromBase(t *testing.T) {
	dir := t.TempDir()

	r, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@test.com"
	require.NoError(t, r.SetConfig(cfg))

	// Initial commit
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644))
	wt, err := r.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Create main branch ref
	head, err := r.Head()
	require.NoError(t, err)
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), head.Hash())
	require.NoError(t, r.Storer.SetReference(ref))

	// Create feature branch with a change
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0644))
	_, err = wt.Add("new.go")
	require.NoError(t, err)
	_, err = wt.Commit("Add new file", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Use Repo.ChangedFilesFromBase
	repo, err := NewRepo(dir)
	require.NoError(t, err)

	files, err := repo.ChangedFilesFromBase("main")
	require.NoError(t, err)
	assert.Contains(t, files, "new.go")
}

func TestIsRepo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	assert.True(t, IsRepo(dir))

	// Subdirectory should NOT be detected as repo root by IsRepo
	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	assert.False(t, IsRepo(subDir))

	tmpDir, err := os.MkdirTemp("", "non-repo-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	assert.False(t, IsRepo(tmpDir))
}

func TestIsInsideRepo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Repo root should be detected
	assert.True(t, IsInsideRepo(dir))

	// Subdirectory inside repo should also be detected
	subDir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	assert.True(t, IsInsideRepo(subDir))

	// Non-repo directory should not be detected
	tmpDir, err := os.MkdirTemp("", "non-repo-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	assert.False(t, IsInsideRepo(tmpDir))
}
