// Package git provides git operations for programmator.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Repo represents a git repository with operations for branch management and commits.
type Repo struct {
	repo     *git.Repository
	workDir  string
	repoRoot string
}

// NewRepo creates a new Repo for the given working directory.
// Returns an error if the directory is not a git repository.
func NewRepo(workDir string) (*Repo, error) {
	r, err := git.PlainOpenWithOptions(workDir, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open git repo at %s: %w", workDir, err)
	}

	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootCmd.Dir = workDir
	rootOut, err := rootCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-parse --show-toplevel at %s: %w", workDir, err)
	}

	return &Repo{repo: r, workDir: workDir, repoRoot: strings.TrimSpace(string(rootOut))}, nil
}

// BranchExists checks if a branch exists (local or remote).
func (r *Repo) BranchExists(branch string) (bool, error) {
	// Check local branch
	_, err := r.repo.Reference(plumbing.NewBranchReferenceName(branch), true)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, fmt.Errorf("check local branch %s: %w", branch, err)
	}

	// Check remote branch
	_, err = r.repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, fmt.Errorf("check remote branch %s: %w", branch, err)
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

	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	ref := plumbing.NewBranchReferenceName(branch)
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: ref,
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("create branch %s: %w", branch, err)
	}
	return nil
}

// CheckoutBranch switches to an existing branch.
func (r *Repo) CheckoutBranch(branch string) error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	ref := plumbing.NewBranchReferenceName(branch)
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: ref,
	})
	if err != nil {
		return fmt.Errorf("checkout branch %s: %w", branch, err)
	}
	return nil
}

// CurrentBranch returns the name of the current branch.
func (r *Repo) CurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("get HEAD: %w", err)
	}
	return head.Name().Short(), nil
}

// Remove stages a file deletion for commit.
func (r *Repo) Remove(file string) error {
	if err := validateRelativePath(file); err != nil {
		return fmt.Errorf("git rm %s: %w", file, err)
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	_, err = wt.Remove(file)
	if err != nil {
		return fmt.Errorf("git rm %s: %w", file, err)
	}
	return nil
}

// Add stages files for commit.
func (r *Repo) Add(files ...string) error {
	if len(files) == 0 {
		return nil
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	for _, file := range files {
		if err := validateRelativePath(file); err != nil {
			return fmt.Errorf("git add %s: %w", file, err)
		}
		if _, err := wt.Add(file); err != nil {
			return fmt.Errorf("git add %s: %w", file, err)
		}
	}
	return nil
}

// Commit creates a commit with the given message.
// Returns nil if there are no staged changes.
func (r *Repo) Commit(message string) error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	// Check if there are staged changes
	hasStagedChanges := false
	for _, s := range status {
		if s.Staging != git.Unmodified && s.Staging != git.Untracked {
			hasStagedChanges = true
			break
		}
	}
	if !hasStagedChanges {
		return nil
	}

	sig := r.commitSignature()
	_, err = wt.Commit(message, &git.CommitOptions{
		Author: sig,
	})
	if err != nil {
		return fmt.Errorf("commit: %w", err)
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

// MoveFile moves a file using git mv equivalent.
func (r *Repo) MoveFile(src, dest string) error {
	if err := validateRelativePath(src); err != nil {
		return fmt.Errorf("git mv source: %w", err)
	}
	if err := validateRelativePath(dest); err != nil {
		return fmt.Errorf("git mv dest: %w", err)
	}
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	_, err = wt.Move(src, dest)
	if err != nil {
		return fmt.Errorf("git mv %s %s: %w", src, dest, err)
	}
	return nil
}

// HasUncommittedChanges returns true if there are uncommitted changes.
func (r *Repo) HasUncommittedChanges() (bool, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("get worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return !status.IsClean(), nil
}

// WorkDir returns the working directory of the repository.
func (r *Repo) WorkDir() string {
	return r.workDir
}

// ChangedFilesFromBase returns files changed between baseBranch and HEAD,
// including staged and unstaged changes, reusing the already-open repository.
func (r *Repo) ChangedFilesFromBase(baseBranch string) ([]string, error) {
	seen := make(map[string]struct{})
	var errs []error

	branchFiles, err := committedDiff(r.repo, baseBranch)
	if err != nil {
		errs = append(errs, fmt.Errorf("committed diff: %w", err))
	}

	wtFiles, err := worktreeChanges(r.repo)
	if err != nil {
		errs = append(errs, fmt.Errorf("worktree changes: %w", err))
	}

	if len(errs) == 2 && len(wtFiles) == 0 && len(branchFiles) == 0 {
		return nil, fmt.Errorf("git diff failed: %v; %v", errs[0], errs[1])
	}

	// Filter gitignored files only from worktree changes (untracked/modified
	// files). Committed diff paths are tracked by git and must not be filtered,
	// otherwise deletions of paths matching .gitignore would be lost.
	filteredWT, filterErr := filterGitIgnored(r.repoRoot, wtFiles)
	if filterErr != nil {
		debug.Logf("filterGitIgnored failed, using unfiltered worktree list: %v", filterErr)
		filteredWT = wtFiles // non-fatal: use unfiltered list
	}

	for _, f := range branchFiles {
		seen[f] = struct{}{}
	}
	for _, f := range filteredWT {
		seen[f] = struct{}{}
	}

	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}

	return files, nil
}

// commitSignature reads user.name and user.email from git config
// (including global/system config), falling back to defaults.
func (r *Repo) commitSignature() *object.Signature {
	name := "programmator"
	email := "programmator@localhost"

	// ConfigScoped merges system + global + local config, unlike Config()
	// which only reads .git/config.
	cfg, err := r.repo.ConfigScoped(config.GlobalScope)
	if err == nil {
		if cfg.User.Name != "" {
			name = cfg.User.Name
		}
		if cfg.User.Email != "" {
			email = cfg.User.Email
		}
	}

	return &object.Signature{
		Name:  name,
		Email: email,
		When:  time.Now(),
	}
}

// validateRelativePath ensures a file path is relative and does not traverse
// outside the repository via ".." components.
func validateRelativePath(path string) error {
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute path not allowed: %s", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}
	return nil
}

// IsRepo checks if the given directory is a git repository root.
// It does not walk parent directories. Use IsInsideRepo for that.
func IsRepo(dir string) bool {
	_, err := git.PlainOpen(dir)
	return err == nil
}

// IsInsideRepo checks if the given directory is inside a git repository,
// walking up parent directories to find a .git folder.
func IsInsideRepo(dir string) bool {
	_, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	return err == nil
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
