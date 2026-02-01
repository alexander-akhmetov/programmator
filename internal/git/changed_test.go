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

func setupChangedTestRepo(t *testing.T) (string, *gogit.Repository) {
	t.Helper()

	dir := t.TempDir()

	r, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@test.com"
	err = r.SetConfig(cfg)
	require.NoError(t, err)

	// Create initial commit on main branch
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644))
	wt, err := r.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Create "main" branch reference pointing to HEAD
	head, err := r.Head()
	require.NoError(t, err)
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), head.Hash())
	require.NoError(t, r.Storer.SetReference(ref))

	return dir, r
}

func TestChangedFiles_CommittedChanges(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create a feature branch
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	// Add a committed file on feature branch
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0644))
	_, err = wt.Add("new.go")
	require.NoError(t, err)
	_, err = wt.Commit("Add new file", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "new.go")
}

func TestChangedFiles_StagedChanges(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Stage a new file without committing
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0644))
	_, err = wt.Add("staged.txt")
	require.NoError(t, err)

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "staged.txt")
}

func TestChangedFiles_UnstagedChanges(t *testing.T) {
	dir, _ := setupChangedTestRepo(t)

	// Modify an existing file without staging
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified\n"), 0644))

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "README.md")
}

func TestChangedFiles_Union(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create feature branch with a committed change
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "committed.go"), []byte("package main\n"), 0644))
	_, err = wt.Add("committed.go")
	require.NoError(t, err)
	_, err = wt.Commit("Add committed file", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Add a staged file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0644))
	_, err = wt.Add("staged.txt")
	require.NoError(t, err)

	// Modify existing file (unstaged)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified\n"), 0644))

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)

	assert.Contains(t, files, "committed.go")
	assert.Contains(t, files, "staged.txt")
	assert.Contains(t, files, "README.md")
}

func TestChangedFiles_NoDuplicates(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create feature branch, commit a change to README, then also modify it unstaged
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Changed\n"), 0644))
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("Modify README", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Now modify it again (unstaged)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Changed again\n"), 0644))

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)

	// README.md should appear exactly once
	count := 0
	for _, f := range files {
		if f == "README.md" {
			count++
		}
	}
	assert.Equal(t, 1, count, "README.md should appear exactly once")
}

func TestChangedFiles_UntrackedFiles(t *testing.T) {
	dir, _ := setupChangedTestRepo(t)

	// Create a new untracked file (not staged)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("new\n"), 0644))

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "untracked.txt")
}

func TestChangedFiles_GitIgnoredFilesExcluded(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create .gitignore that ignores *.cache files
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.cache\n"), 0644))
	_, err = wt.Add(".gitignore")
	require.NoError(t, err)
	_, err = wt.Commit("Add gitignore", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Create an ignored file and a normal untracked file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.cache"), []byte("cached\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.txt"), []byte("real\n"), 0644))

	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)
	assert.NotContains(t, files, "data.cache", "gitignored file should be excluded")
	assert.Contains(t, files, "real.txt", "non-ignored file should be included")
}

func TestChangedFiles_CommittedFileMatchingGitignoreIncluded(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create feature branch
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	// Commit a .cache file (tracked by git)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.cache"), []byte("tracked\n"), 0644))
	_, err = wt.Add("data.cache")
	require.NoError(t, err)
	_, err = wt.Commit("Add cache file", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Now add .gitignore that matches *.cache
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.cache\n"), 0644))
	_, err = wt.Add(".gitignore")
	require.NoError(t, err)
	_, err = wt.Commit("Add gitignore", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Committed diff paths should NOT be filtered even if they match gitignore
	files, err := ChangedFiles(dir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "data.cache", "committed file matching gitignore should still be included")
	assert.Contains(t, files, ".gitignore")
}

func TestFilterGitIgnored_SpecialCharacters(t *testing.T) {
	dir := t.TempDir()

	_, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.cache\n"), 0644))

	files := []string{"file with spaces.go", "normal.go", "data.cache"}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644))
	}

	result, err := filterGitIgnored(dir, files)
	require.NoError(t, err)
	assert.Equal(t, []string{"file with spaces.go", "normal.go"}, result)
}

func TestFilterGitIgnored_NonGitDir(t *testing.T) {
	dir := t.TempDir()

	// No git repo initialized - filterGitIgnored should return an error
	files := []string{"main.go", "README.md"}
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644))
	}

	_, err := filterGitIgnored(dir, files)
	assert.Error(t, err, "filterGitIgnored should fail in a non-git directory")
}

func TestFilterGitIgnored(t *testing.T) {
	tests := []struct {
		name     string
		ignore   string
		files    []string
		expected []string
	}{
		{
			name:     "empty list",
			ignore:   "*.cache",
			files:    nil,
			expected: nil,
		},
		{
			name:     "no files ignored",
			ignore:   "*.cache",
			files:    []string{"main.go", "README.md"},
			expected: []string{"main.go", "README.md"},
		},
		{
			name:     "some files ignored",
			ignore:   "*.cache\n*.tmp\n",
			files:    []string{"main.go", "data.cache", "temp.tmp", "README.md"},
			expected: []string{"main.go", "README.md"},
		},
		{
			name:     "all files ignored",
			ignore:   "*.cache",
			files:    []string{"a.cache", "b.cache"},
			expected: []string{},
		},
		{
			name:     "directory pattern",
			ignore:   ".aider*\n",
			files:    []string{".aider.chat.md", ".aider.input", "main.go"},
			expected: []string{"main.go"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			// Init a git repo
			_, err := gogit.PlainInit(dir, false)
			require.NoError(t, err)

			// Write .gitignore
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.ignore), 0644))

			// Create the files so git check-ignore can test them
			for _, f := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644))
			}

			result, err := filterGitIgnored(dir, tc.files)
			require.NoError(t, err)

			if tc.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestChangedFilesFromBase_CommittedAndWorktree(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create feature branch with a committed change
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "committed.go"), []byte("package main\n"), 0644))
	_, err = wt.Add("committed.go")
	require.NoError(t, err)
	_, err = wt.Commit("Add committed file", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Add an unstaged file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unstaged.txt"), []byte("unstaged\n"), 0644))

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	files, err := repo.ChangedFilesFromBase("main")
	require.NoError(t, err)
	assert.Contains(t, files, "committed.go")
	assert.Contains(t, files, "unstaged.txt")
}

func TestChangedFilesFromBase_GitIgnoredExcluded(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create .gitignore
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.cache\n"), 0644))
	_, err = wt.Add(".gitignore")
	require.NoError(t, err)
	_, err = wt.Commit("Add gitignore", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Create an ignored file and a normal file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.cache"), []byte("cached\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.txt"), []byte("real\n"), 0644))

	repo, err := NewRepo(dir)
	require.NoError(t, err)

	files, err := repo.ChangedFilesFromBase("main")
	require.NoError(t, err)
	assert.NotContains(t, files, "data.cache", "gitignored file should be excluded")
	assert.Contains(t, files, "real.txt", "non-ignored file should be included")
}

func TestChangedFiles_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := ChangedFiles(dir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open git repo")
}

func TestChangedFiles_MissingBaseBranch(t *testing.T) {
	dir, _ := setupChangedTestRepo(t)

	// Should still return worktree changes even if base branch resolution fails
	// (only errors if BOTH sources fail)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified\n"), 0644))

	files, err := ChangedFiles(dir, "nonexistent-branch")
	require.NoError(t, err)
	assert.Contains(t, files, "README.md")
}

func TestWorktreeChanges_StagedAndUnstaged(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Stage a new file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0644))
	_, err = wt.Add("staged.txt")
	require.NoError(t, err)

	// Modify existing file without staging
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Modified\n"), 0644))

	files, err := worktreeChanges(r)
	require.NoError(t, err)
	assert.Contains(t, files, "staged.txt")
	assert.Contains(t, files, "README.md")
}

func TestWorktreeChanges_NoChanges(t *testing.T) {
	_, r := setupChangedTestRepo(t)

	files, err := worktreeChanges(r)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestCommittedDiff_BranchDiverge(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Create feature branch
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	// Add file on feature
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.go"), []byte("package feature\n"), 0644))
	_, err = wt.Add("feature.go")
	require.NoError(t, err)
	_, err = wt.Commit("Feature commit", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	files, err := committedDiff(r, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "feature.go")
}

func TestCommittedDiff_MissingBranch(t *testing.T) {
	_, r := setupChangedTestRepo(t)

	_, err := committedDiff(r, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resolve base branch")
}

func TestCommittedDiff_DeletedFile(t *testing.T) {
	dir, r := setupChangedTestRepo(t)

	wt, err := r.Worktree()
	require.NoError(t, err)

	// Add another file on main first
	require.NoError(t, os.WriteFile(filepath.Join(dir, "delete-me.txt"), []byte("bye\n"), 0644))
	_, err = wt.Add("delete-me.txt")
	require.NoError(t, err)
	_, err = wt.Commit("Add file to delete", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	// Update main branch ref
	head, err := r.Head()
	require.NoError(t, err)
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), head.Hash())
	require.NoError(t, r.Storer.SetReference(ref))

	// Create feature branch and delete the file
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(dir, "delete-me.txt")))
	_, err = wt.Remove("delete-me.txt")
	require.NoError(t, err)
	_, err = wt.Commit("Delete file", &gogit.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@test.com", When: time.Now()},
	})
	require.NoError(t, err)

	files, err := committedDiff(r, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "delete-me.txt")
}
