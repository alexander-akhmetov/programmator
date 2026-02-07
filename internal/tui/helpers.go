package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/alexander-akhmetov/programmator/internal/llm"
)

// applySkipPermissions ensures --dangerously-skip-permissions is present in
// ExtraFlags when the caller requests it (via guard-mode or explicit flag).
func applySkipPermissions(cfg *llm.ExecutorConfig) {
	if cfg.ExtraFlags == "" {
		cfg.ExtraFlags = "--dangerously-skip-permissions"
	} else if !strings.Contains(cfg.ExtraFlags, "--dangerously-skip-permissions") {
		cfg.ExtraFlags += " --dangerously-skip-permissions"
	}
}

// resolveGuardMode checks whether dcg is available and adjusts the executor
// config accordingly. Returns the effective guard-mode value.
// If dcg is not found, guard mode is disabled and a warning is printed.
func resolveGuardMode(guardMode bool, cfg *llm.ExecutorConfig) bool {
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
