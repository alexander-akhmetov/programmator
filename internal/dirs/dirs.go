// Package dirs provides XDG Base Directory Specification compliant paths
// for all programmator directories.
package dirs

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the programmator configuration directory.
// Resolution order: XDG_CONFIG_HOME/programmator > ~/.config/programmator.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "programmator")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "programmator")
	}
	return filepath.Join(home, ".config", "programmator")
}

// StateDir returns the programmator state directory.
// Resolution order: PROGRAMMATOR_STATE_DIR > XDG_STATE_HOME/programmator > ~/.local/state/programmator.
func StateDir() string {
	if dir := os.Getenv("PROGRAMMATOR_STATE_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "programmator")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "state", "programmator")
	}
	return filepath.Join(home, ".local", "state", "programmator")
}

// LogsDir returns the programmator logs directory (StateDir/logs).
func LogsDir() string {
	return filepath.Join(StateDir(), "logs")
}
