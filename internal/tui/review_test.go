package tui

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/git"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/review"
)

func TestReviewCmdDefinition(t *testing.T) {
	assert.Equal(t, "review", reviewCmd.Use)
	assert.NotEmpty(t, reviewCmd.Short)
	assert.NotEmpty(t, reviewCmd.Long)
}

func TestReviewCmdFlags(t *testing.T) {
	flags := reviewCmd.Flags()

	baseFlag := flags.Lookup("base")
	require.NotNil(t, baseFlag)
	assert.Equal(t, "main", baseFlag.DefValue)

	dirFlag := flags.Lookup("dir")
	require.NotNil(t, dirFlag)
	assert.Equal(t, "d", dirFlag.Shorthand)
}

func TestReviewCmdHelp(t *testing.T) {
	assert.Contains(t, reviewCmd.Long, "Run code review")
	assert.Contains(t, reviewCmd.Long, "--base")
	assert.Contains(t, reviewCmd.Long, "programmator review")
}

func TestIsGitRepo(t *testing.T) {
	// Test with actual git repo (this test is in a git repo)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	if !git.IsInsideRepo(cwd) {
		t.Skip("Not in a git repository")
	}

	// Current directory should be a git repo
	assert.True(t, git.IsInsideRepo(cwd))

	// A fresh temp directory should not be a git repo
	nonRepoDir := t.TempDir()
	assert.False(t, git.IsInsideRepo(nonRepoDir))
}

func TestGetChangedFiles(t *testing.T) {
	// This test requires a git repo, skip if not available
	cwd, err := os.Getwd()
	require.NoError(t, err)

	if !git.IsInsideRepo(cwd) {
		t.Skip("Not in a git repository")
	}

	// Test with HEAD (should work even if no changes)
	files, err := git.ChangedFiles(cwd, "HEAD")
	if err != nil {
		t.Skipf("ChangedFiles returned error (may be empty repo): %v", err)
	}
	assert.NotNil(t, files, "files should be non-nil when no error")
}

func TestGetChangedFilesNonGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := git.ChangedFiles(tmpDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "open git repo")
}

func TestRunReviewNotGitRepo(t *testing.T) {
	// Save and restore the working dir flag
	oldWorkingDir := reviewWorkingDir
	defer func() { reviewWorkingDir = oldWorkingDir }()

	tmpDir := t.TempDir()
	reviewWorkingDir = tmpDir

	err := runReview(nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestRunReviewNoChanges(t *testing.T) {
	// Create a temp git repo with no changes
	tmpDir := t.TempDir()
	setupTestGitRepo(t, tmpDir)

	oldWorkingDir := reviewWorkingDir
	oldBaseBranch := reviewBaseBranch
	defer func() {
		reviewWorkingDir = oldWorkingDir
		reviewBaseBranch = oldBaseBranch
	}()

	reviewWorkingDir = tmpDir
	reviewBaseBranch = "HEAD" // Compare HEAD to HEAD = no changes

	// Should succeed with no changes message
	err := runReview(nil, nil)
	assert.NoError(t, err)
}

func TestFormatReviewDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"seconds only", 45 * time.Second, "45s"},
		{"minutes and seconds", 2*time.Minute + 30*time.Second, "2m30s"},
		{"just minutes", 5 * time.Minute, "5m0s"},
		{"zero", 0, "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatReviewDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// setupTestGitRepo creates a minimal git repo for testing.
func TestGetChangedFilesWithBranch(t *testing.T) {
	// Create a git repo with branches
	tmpDir := t.TempDir()
	setupTestGitRepoWithBranch(t, tmpDir)

	// Test getting changed files vs main
	files, err := git.ChangedFiles(tmpDir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "new_file.go")
}

func TestReviewCmdRegistered(t *testing.T) {
	// Verify review command is registered with root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "review" {
			found = true
			break
		}
	}
	assert.True(t, found, "review command should be registered with root")
}

func TestPrintReviewOnlySummaryPassed(t *testing.T) {
	output := captureStdout(t, func() {
		result := &loop.ReviewOnlyResult{
			Passed:      true,
			Iterations:  1,
			TotalIssues: 0,
			FilesFixed:  []string{},
			Duration:    30 * time.Second,
			CommitsMade: 0,
		}
		printReviewOnlySummary(result)
	})

	assert.Contains(t, output, "PASSED")
	assert.Contains(t, output, "1")   // iterations
	assert.Contains(t, output, "0")   // issues
	assert.Contains(t, output, "30s") // duration
}

func TestPrintReviewOnlySummaryFailed(t *testing.T) {
	output := captureStdout(t, func() {
		result := &loop.ReviewOnlyResult{
			Passed:      false,
			Iterations:  3,
			TotalIssues: 5,
			FilesFixed:  []string{"main.go", "util.go"},
			Duration:    2*time.Minute + 15*time.Second,
			CommitsMade: 2,
		}
		printReviewOnlySummary(result)
	})

	assert.Contains(t, output, "FAILED")
	assert.Contains(t, output, "3")       // iterations
	assert.Contains(t, output, "5")       // issues
	assert.Contains(t, output, "2m15s")   // duration
	assert.Contains(t, output, "main.go") // files fixed
	assert.Contains(t, output, "util.go")
}

func TestPrintReviewOnlySummaryWithFinalReview(t *testing.T) {
	output := captureStdout(t, func() {
		result := &loop.ReviewOnlyResult{
			Passed:      false,
			Iterations:  2,
			TotalIssues: 1,
			FilesFixed:  []string{},
			Duration:    45 * time.Second,
			FinalReview: &review.RunResult{
				Passed:      false,
				TotalIssues: 1,
				Results: []*review.Result{
					{
						AgentName: "test_agent",
						Issues: []review.Issue{
							{
								File:        "test.go",
								Line:        42,
								Severity:    review.SeverityHigh,
								Description: "Test issue",
							},
						},
					},
				},
			},
		}
		printReviewOnlySummary(result)
	})

	assert.Contains(t, output, "FAILED")
	assert.Contains(t, output, "Remaining issues")
	assert.Contains(t, output, "test.go")
}

// setupTestGitRepo creates a minimal git repo for testing.
func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	r, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@test.com"
	err = r.SetConfig(cfg)
	require.NoError(t, err)

	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))

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
}

// setupTestGitRepoWithBranch creates a git repo with main and feature branches.
func setupTestGitRepoWithBranch(t *testing.T, dir string) {
	t.Helper()

	r, err := gogit.PlainInitWithOptions(dir, &gogit.PlainInitOptions{
		InitOptions: gogit.InitOptions{
			DefaultBranch: plumbing.NewBranchReferenceName("main"),
		},
		Bare: false,
	})
	require.NoError(t, err)

	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@test.com"
	err = r.SetConfig(cfg)
	require.NoError(t, err)

	// Create initial commit on main
	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))

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

	// Create feature branch
	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

	// Add a new file on feature branch
	newFile := filepath.Join(dir, "new_file.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package main\n"), 0644))

	_, err = wt.Add("new_file.go")
	require.NoError(t, err)

	_, err = wt.Commit("Add new file", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
}

// captureStdout redirects os.Stdout during fn execution and returns the captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	data, err := io.ReadAll(r)
	require.NoError(t, err)

	return string(data)
}
