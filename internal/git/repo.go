// Package git provides git operations for programmator.
package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Repo represents a git repository with operations for branch management and commits.
type Repo struct {
	workDir string
}

// NewRepo creates a new Repo for the given working directory.
func NewRepo(workDir string) *Repo {
	return &Repo{workDir: workDir}
}

// BranchExists checks if a branch exists (local or remote).
func (r *Repo) BranchExists(branch string) (bool, error) {
	// Check local branches
	cmd := exec.Command("git", "-C", r.workDir, "rev-parse", "--verify", branch)
	if err := cmd.Run(); err == nil {
		return true, nil
	}

	// Check remote branches
	cmd = exec.Command("git", "-C", r.workDir, "rev-parse", "--verify", "origin/"+branch)
	if err := cmd.Run(); err == nil {
		return true, nil
	}

	return false, nil
}

// CreateBranch creates a new branch and switches to it.
// If the branch already exists, it just checks it out.
func (r *Repo) CreateBranch(branch string) error {
	exists, err := r.BranchExists(branch)
	if err != nil {
		return fmt.Errorf("check branch exists: %w", err)
	}

	if exists {
		return r.CheckoutBranch(branch)
	}

	cmd := exec.Command("git", "-C", r.workDir, "checkout", "-b", branch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create branch %s: %w: %s", branch, err, string(output))
	}
	return nil
}

// CheckoutBranch switches to an existing branch.
func (r *Repo) CheckoutBranch(branch string) error {
	cmd := exec.Command("git", "-C", r.workDir, "checkout", branch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checkout branch %s: %w: %s", branch, err, string(output))
	}
	return nil
}

// CurrentBranch returns the name of the current branch.
func (r *Repo) CurrentBranch() (string, error) {
	cmd := exec.Command("git", "-C", r.workDir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Add stages files for commit.
func (r *Repo) Add(files ...string) error {
	if len(files) == 0 {
		return nil
	}
	args := append([]string{"-C", r.workDir, "add", "--"}, files...)
	cmd := exec.Command("git", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w: %s", err, string(output))
	}
	return nil
}

// Commit creates a commit with the given message.
// Returns nil if there are no staged changes.
func (r *Repo) Commit(message string) error {
	// Check if there are staged changes
	cmd := exec.Command("git", "-C", r.workDir, "diff", "--cached", "--quiet")
	if err := cmd.Run(); err == nil {
		// No staged changes, nothing to commit
		return nil
	}

	cmd = exec.Command("git", "-C", r.workDir, "commit", "-m", message)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("commit: %w: %s", err, string(output))
	}
	return nil
}

// AddAndCommit stages files and commits them with the given message.
// Returns nil if there are no changes to commit.
func (r *Repo) AddAndCommit(files []string, message string) error {
	if len(files) == 0 {
		return nil
	}

	if err := r.Add(files...); err != nil {
		return err
	}
	return r.Commit(message)
}

// MoveFile moves a file using git mv.
func (r *Repo) MoveFile(src, dest string) error {
	cmd := exec.Command("git", "-C", r.workDir, "mv", src, dest)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git mv %s %s: %w: %s", src, dest, err, string(output))
	}
	return nil
}

// HasUncommittedChanges returns true if there are uncommitted changes.
func (r *Repo) HasUncommittedChanges() (bool, error) {
	cmd := exec.Command("git", "-C", r.workDir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// BranchNameFromSource generates a branch name from a source identifier.
// For plan files: "programmator/<filename-without-ext>"
// For tickets: "programmator/<ticket-id>"
func BranchNameFromSource(sourceID string, isPlan bool) string {
	var slug string
	if isPlan {
		// Extract filename without extension
		base := filepath.Base(sourceID)
		ext := filepath.Ext(base)
		slug = strings.TrimSuffix(base, ext)
	} else {
		slug = sourceID
	}

	// Sanitize for git branch name: replace invalid chars with dashes
	slug = sanitizeBranchName(slug)

	return "programmator/" + slug
}

// sanitizeBranchName makes a string safe for use as a git branch name.
func sanitizeBranchName(s string) string {
	// Git branch naming rules:
	// - Cannot contain: space, ~, ^, :, ?, *, [, \
	// - Cannot start/end with / or .
	// - Cannot contain consecutive slashes or end with .lock

	// Replace common invalid characters with dashes
	invalidChars := regexp.MustCompile(`[~^:?*\[\]\\@{}\s]+`)
	s = invalidChars.ReplaceAllString(s, "-")

	// Replace consecutive dashes
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")

	// Trim leading/trailing dashes and dots
	s = strings.Trim(s, "-.")

	// Ensure not empty
	if s == "" {
		s = "work"
	}

	return s
}
