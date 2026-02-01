package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// ChangedFiles returns the list of files changed between baseBranch and HEAD,
// including staged and unstaged changes. It unions:
//   - committed branch diff (merge-base of baseBranch and HEAD)
//   - staged changes
//   - unstaged working directory changes
//
// Returns an error only if all sources fail (e.g. not a git repo).
func ChangedFiles(workingDir, baseBranch string) ([]string, error) {
	r, err := NewRepo(workingDir)
	if err != nil {
		return nil, fmt.Errorf("open git repo: %w", err)
	}
	return r.ChangedFilesFromBase(baseBranch)
}

// committedDiff returns files changed between baseBranch and HEAD.
// It tries to find the merge-base (three-dot diff equivalent), falling back
// to a direct two-commit diff.
func committedDiff(repo *gogit.Repository, baseBranch string) ([]string, error) {
	// Resolve HEAD
	headRef, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	headCommit, err := repo.CommitObject(headRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("get HEAD commit: %w", err)
	}

	// Resolve base branch
	baseRef, err := repo.Reference(plumbing.NewBranchReferenceName(baseBranch), true)
	if err != nil {
		// Try as remote ref
		baseRef, err = repo.Reference(plumbing.NewRemoteReferenceName("origin", baseBranch), true)
		if err != nil {
			return nil, fmt.Errorf("resolve base branch %s: %w", baseBranch, err)
		}
	}
	baseCommit, err := repo.CommitObject(baseRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("get base commit: %w", err)
	}

	// Try merge-base (three-dot equivalent)
	mergeBase, err := headCommit.MergeBase(baseCommit)
	var diffFromCommit = baseCommit
	if err == nil && len(mergeBase) > 0 {
		diffFromCommit = mergeBase[0]
	}

	// Get the diff
	fromTree, err := diffFromCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get base tree: %w", err)
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("get head tree: %w", err)
	}

	changes, err := fromTree.Diff(headTree)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}

	var files []string
	for _, change := range changes {
		name := change.To.Name
		if name == "" {
			name = change.From.Name
		}
		if name != "" {
			files = append(files, name)
		}
	}

	return files, nil
}

// worktreeChanges returns files with staged or unstaged changes.
func worktreeChanges(repo *gogit.Repository) ([]string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("get worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("get worktree status: %w", err)
	}

	var files []string
	for path, s := range status {
		if s.Staging != gogit.Unmodified || s.Worktree != gogit.Unmodified {
			files = append(files, path)
		}
	}

	return files, nil
}

// filterGitIgnored removes gitignored files from the list by running
// `git check-ignore -z --stdin` in the given repo root directory.
// Uses NUL-delimited I/O to correctly handle filenames with special characters.
func filterGitIgnored(repoRoot string, files []string) ([]string, error) {
	if len(files) == 0 {
		return files, nil
	}

	input := strings.Join(files, "\x00") + "\x00"
	cmd := exec.Command("git", "check-ignore", "--stdin", "-z")
	cmd.Dir = repoRoot
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// git check-ignore exits 1 when no files are ignored — not an error for us.
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return files, nil
		}
		return nil, fmt.Errorf("git check-ignore: %w (stderr: %s)", err, stderr.String())
	}

	ignored := make(map[string]struct{})
	for entry := range strings.SplitSeq(stdout.String(), "\x00") {
		if entry != "" {
			ignored[entry] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(files))
	for _, f := range files {
		if _, ok := ignored[f]; !ok {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}
