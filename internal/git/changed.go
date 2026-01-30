package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ChangedFiles returns the list of files changed between baseBranch and HEAD,
// including staged and unstaged changes. It unions:
//   - committed branch diff (baseBranch...HEAD or baseBranch fallback)
//   - staged changes (--cached)
//   - unstaged working directory changes
//
// Returns an error only if all three sources fail (e.g. not a git repo).
func ChangedFiles(workingDir, baseBranch string) ([]string, error) {
	seen := make(map[string]struct{})
	failures := 0

	// 1. Committed branch diff
	branchFiles, err := gitDiffFiles(workingDir, baseBranch+"...HEAD")
	if err != nil {
		branchFiles, err = gitDiffFiles(workingDir, baseBranch)
	}
	if err != nil {
		failures++
	}
	for _, f := range branchFiles {
		seen[f] = struct{}{}
	}

	// 2. Staged changes
	stagedFiles, err := gitDiffFiles(workingDir, "--cached")
	if err != nil {
		failures++
	}
	for _, f := range stagedFiles {
		seen[f] = struct{}{}
	}

	// 3. Unstaged working directory changes
	unstagedFiles, err := gitDiffFiles(workingDir)
	if err != nil {
		failures++
	}
	for _, f := range unstagedFiles {
		seen[f] = struct{}{}
	}

	if failures == 3 {
		return nil, fmt.Errorf("git diff failed: not a git repository or all diff commands failed")
	}

	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}

	return files, nil
}

// gitDiffFiles runs git diff --name-only with the given args and returns the file list.
func gitDiffFiles(workingDir string, args ...string) ([]string, error) {
	cmdArgs := append([]string{"-C", workingDir, "diff", "--name-only"}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}
