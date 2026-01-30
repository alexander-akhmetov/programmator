package cmd

import (
	"os"
	"testing"
)

func TestIsProcessRunning(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		expected bool
	}{
		{
			name:     "current process",
			pid:      os.Getpid(),
			expected: true,
		},
		{
			name:     "non-existent process",
			pid:      999999999,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProcessRunning(tt.pid)
			if got != tt.expected {
				t.Errorf("isProcessRunning(%d) = %v, want %v", tt.pid, got, tt.expected)
			}
		})
	}
}

func TestSessionFilePath(t *testing.T) {
	originalStateDir := os.Getenv("PROGRAMMATOR_STATE_DIR")
	defer os.Setenv("PROGRAMMATOR_STATE_DIR", originalStateDir)

	os.Setenv("PROGRAMMATOR_STATE_DIR", "/custom/state")
	path := sessionFilePath()
	if path != "/custom/state/session.json" {
		t.Errorf("sessionFilePath() = %q, want %q", path, "/custom/state/session.json")
	}

	os.Unsetenv("PROGRAMMATOR_STATE_DIR")
	path = sessionFilePath()
	if path == "" {
		t.Error("sessionFilePath() should not be empty when PROGRAMMATOR_STATE_DIR is unset")
	}
}
