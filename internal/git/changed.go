package git

import (
	"fmt"

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
	repo, err := gogit.PlainOpenWithOptions(workingDir, &gogit.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open git repo: %w", err)
	}

	seen := make(map[string]struct{})
	var errs []error

	// 1. Committed branch diff
	branchFiles, err := committedDiff(repo, baseBranch)
	if err != nil {
		errs = append(errs, fmt.Errorf("committed diff: %w", err))
	}
	for _, f := range branchFiles {
		seen[f] = struct{}{}
	}

	// 2. Staged + unstaged changes from worktree status
	wtFiles, err := worktreeChanges(repo)
	if err != nil {
		errs = append(errs, fmt.Errorf("worktree changes: %w", err))
	}
	for _, f := range wtFiles {
		seen[f] = struct{}{}
	}

	if len(errs) == 2 && len(seen) == 0 {
		return nil, fmt.Errorf("git diff failed: %v; %v", errs[0], errs[1])
	}

	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}

	return files, nil
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
