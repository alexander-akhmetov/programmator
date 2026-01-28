package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ChangedFiles returns the list of files changed between baseBranch and HEAD.
// It tries a three-dot diff first (changes since branching), falling back to two-dot diff.
func ChangedFiles(workingDir, baseBranch string) ([]string, error) {
	cmd := exec.Command("git", "-C", workingDir, "diff", "--name-only", baseBranch+"...HEAD")
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "-C", workingDir, "diff", "--name-only", baseBranch)
		out, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
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
