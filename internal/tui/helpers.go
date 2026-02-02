package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// applySkipPermissions ensures --dangerously-skip-permissions is present in
// ClaudeFlags when the caller requests it (via guard-mode or explicit flag).
func applySkipPermissions(cfg *safety.Config) {
	if cfg.Claude.Flags == "" {
		cfg.Claude.Flags = "--dangerously-skip-permissions"
	} else if !strings.Contains(cfg.Claude.Flags, "--dangerously-skip-permissions") {
		cfg.Claude.Flags += " --dangerously-skip-permissions"
	}
}

// resolveGuardMode checks whether dcg is available and adjusts the safety
// config accordingly. Returns the effective guard-mode value.
// If dcg is not found, guard mode is disabled and a warning is printed.
func resolveGuardMode(guardMode bool, cfg *safety.Config) bool {
	if !guardMode {
		return false
	}
	if _, err := exec.LookPath("dcg"); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: dcg not found, falling back to interactive permissions. Install: https://github.com/Dicklesworthstone/destructive_command_guard")
		return false
	}
	applySkipPermissions(cfg)
	return true
}

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
