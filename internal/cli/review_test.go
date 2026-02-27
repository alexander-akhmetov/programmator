package cli

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
	cwd, err := os.Getwd()
	require.NoError(t, err)

	if !git.IsInsideRepo(cwd) {
		t.Skip("Not in a git repository")
	}

	assert.True(t, git.IsInsideRepo(cwd))

	nonRepoDir := t.TempDir()
	assert.False(t, git.IsInsideRepo(nonRepoDir))
}

func TestGetChangedFiles(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	if !git.IsInsideRepo(cwd) {
		t.Skip("Not in a git repository")
	}

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
	oldWorkingDir := reviewWorkDir
	defer func() { reviewWorkDir = oldWorkingDir }()

	tmpDir := t.TempDir()
	reviewWorkDir = tmpDir

	err := runReview(nil, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestRunReviewNoChanges(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestGitRepo(t, tmpDir)

	oldWorkingDir := reviewWorkDir
	oldBaseBranch := reviewBaseBranch
	defer func() {
		reviewWorkDir = oldWorkingDir
		reviewBaseBranch = oldBaseBranch
	}()

	reviewWorkDir = tmpDir
	reviewBaseBranch = "HEAD"

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

func TestGetChangedFilesWithBranch(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestGitRepoWithBranch(t, tmpDir)

	files, err := git.ChangedFiles(tmpDir, "main")
	require.NoError(t, err)
	assert.Contains(t, files, "new_file.go")
}

func TestReviewCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "review" {
			found = true
			break
		}
	}
	assert.True(t, found, "standalone review command should be registered")
}

func TestPrintReviewSummary(t *testing.T) {
	tests := []struct {
		name     string
		result   *review.RunResult
		contains []string
	}{
		{
			name: "passed",
			result: &review.RunResult{
				Passed:      true,
				Iteration:   1,
				TotalIssues: 0,
				Duration:    30 * time.Second,
			},
			contains: []string{"PASSED", "1", "0", "30s"},
		},
		{
			name: "failed",
			result: &review.RunResult{
				Passed:      false,
				Iteration:   3,
				TotalIssues: 5,
				Duration:    2*time.Minute + 15*time.Second,
			},
			contains: []string{"FAILED", "3", "5", "2m15s"},
		},
		{
			name: "failed with remaining issues",
			result: &review.RunResult{
				Passed:      false,
				Iteration:   2,
				TotalIssues: 1,
				Duration:    45 * time.Second,
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
			contains: []string{"FAILED", "Remaining issues", "test.go"},
		},
		{
			name: "passed with commits and files",
			result: &review.RunResult{
				Passed:      true,
				Iteration:   3,
				TotalIssues: 4,
				Duration:    5*time.Minute + 30*time.Second,
			},
			contains: []string{"PASSED", "3", "4", "5m30s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				printReviewSummary(tt.result)
			})

			for _, s := range tt.contains {
				assert.Contains(t, output, s)
			}
		})
	}
}

func TestReviewCmdAllFlagDefaults(t *testing.T) {
	flags := reviewCmd.Flags()

	tests := []struct {
		name     string
		flag     string
		defValue string
	}{
		{"base branch", "base", "main"},
		{"working dir", "dir", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := flags.Lookup(tc.flag)
			require.NotNil(t, f, "flag %s should exist", tc.flag)
			assert.Equal(t, tc.defValue, f.DefValue, "flag %s default", tc.flag)
		})
	}
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

	err = wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
	})
	require.NoError(t, err)

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
