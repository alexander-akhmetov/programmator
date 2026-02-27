package tui

import (
	"fmt"
	"os"
)

// resolveWorkingDir returns the provided dir or falls back to the current
// working directory.
func resolveWorkingDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	return wd, nil
}
