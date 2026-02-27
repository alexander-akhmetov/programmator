package cli

import (
	"fmt"
	"os"
	"time"
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

func formatElapsed(d time.Duration) string {
	total := int(d.Seconds())
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	m := total / 60
	s := total % 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := m / 60
	m %= 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func formatMemory(kb int64) string {
	if kb >= 1024*1024 {
		return fmt.Sprintf("%.1fGB", float64(kb)/(1024*1024))
	}
	if kb >= 1024 {
		return fmt.Sprintf("%.0fMB", float64(kb)/1024)
	}
	return fmt.Sprintf("%dKB", kb)
}
