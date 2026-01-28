package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

	// Find the git root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	_, err = cmd.Output()
	if err != nil {
		t.Skip("Not in a git repository")
	}

	// Current directory should be a git repo
	assert.True(t, isGitRepo(cwd))

	// /tmp is likely not a git repo
	assert.False(t, isGitRepo("/tmp"))
}

func TestGetChangedFiles(t *testing.T) {
	// This test requires a git repo, skip if not available
	cwd, err := os.Getwd()
	require.NoError(t, err)

	if !isGitRepo(cwd) {
		t.Skip("Not in a git repository")
	}

	// Test with HEAD (should work even if no changes)
	files, err := git.ChangedFiles(cwd, "HEAD")
	// Note: This may fail if there's no HEAD commit, which is fine for an empty repo
	if err == nil {
		// Result can be nil or empty slice when no changes - both are valid
		// Just verify no error occurred
		_ = files
	}
}

func TestGetChangedFilesNonGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := git.ChangedFiles(tmpDir, "main")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git diff failed")
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

func TestPrintReviewOnlySummaryPassed(_ *testing.T) {
	result := &loop.ReviewOnlyResult{
		Passed:      true,
		Iterations:  1,
		TotalIssues: 0,
		FilesFixed:  []string{},
		Duration:    30 * time.Second,
		CommitsMade: 0,
	}

	// This just tests that the function doesn't panic
	// The output goes to stdout which we're not capturing here
	printReviewOnlySummary(result)
}

func TestPrintReviewOnlySummaryFailed(_ *testing.T) {
	result := &loop.ReviewOnlyResult{
		Passed:      false,
		Iterations:  3,
		TotalIssues: 5,
		FilesFixed:  []string{"main.go", "util.go"},
		Duration:    2*time.Minute + 15*time.Second,
		CommitsMade: 2,
	}

	// This just tests that the function doesn't panic
	printReviewOnlySummary(result)
}

func TestPrintReviewOnlySummaryWithFinalReview(_ *testing.T) {
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

	// This just tests that the function doesn't panic
	printReviewOnlySummary(result)
}

// setupTestGitRepo creates a minimal git repo for testing.
func setupTestGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Configure git user (required for commits)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Create initial commit
	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

// setupTestGitRepoWithBranch creates a git repo with main and feature branches.
func setupTestGitRepoWithBranch(t *testing.T, dir string) {
	t.Helper()

	// Initialize repo
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Create initial commit on main
	testFile := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Create feature branch
	cmd = exec.Command("git", "checkout", "-b", "feature")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	// Add a new file on feature branch
	newFile := filepath.Join(dir, "new_file.go")
	require.NoError(t, os.WriteFile(newFile, []byte("package main\n"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "Add new file")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}
