package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "PROGRAMMATOR_STATE_DIR takes precedence",
			envVars:  map[string]string{"PROGRAMMATOR_STATE_DIR": "/custom/state", "XDG_STATE_HOME": "/xdg/state"},
			expected: "/custom/state/session.json",
		},
		{
			name:     "XDG_STATE_HOME used when PROGRAMMATOR_STATE_DIR unset",
			envVars:  map[string]string{"PROGRAMMATOR_STATE_DIR": "", "XDG_STATE_HOME": "/xdg/state"},
			expected: filepath.Join("/xdg/state", "programmator", "session.json"),
		},
		{
			name:    "falls back to default when both unset",
			envVars: map[string]string{"PROGRAMMATOR_STATE_DIR": "", "XDG_STATE_HOME": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			path := sessionFilePath()
			if tt.expected != "" {
				assert.Equal(t, tt.expected, path)
			} else {
				assert.NotEmpty(t, path)
			}
		})
	}
}

func TestRunStatus(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		wantErr     bool
	}{
		{
			name:        "no session file",
			fileContent: "", // empty means don't create file
		},
		{
			name:        "corrupted JSON file",
			fileContent: "not valid json{{{",
		},
		{
			name: "stale session with dead PID",
			fileContent: mustMarshal(sessionInfo{
				TicketID:   "test-123",
				WorkingDir: "/tmp/test",
				StartedAt:  "2024-01-01T00:00:00Z",
				PID:        999999999,
			}),
		},
		{
			name: "active session with current PID",
			fileContent: mustMarshal(sessionInfo{
				TicketID:   "test-456",
				WorkingDir: "/tmp/test",
				StartedAt:  "2024-01-01T00:00:00Z",
				PID:        os.Getpid(),
			}),
		},
		{
			name: "active session with invalid timestamp",
			fileContent: mustMarshal(sessionInfo{
				TicketID:   "test-789",
				WorkingDir: "/tmp/test",
				StartedAt:  "not-a-timestamp",
				PID:        os.Getpid(),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("PROGRAMMATOR_STATE_DIR", tmpDir)

			if tt.fileContent != "" {
				err := os.WriteFile(filepath.Join(tmpDir, "session.json"), []byte(tt.fileContent), 0o600)
				require.NoError(t, err)
			}

			err := runStatus(nil, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// For corrupted and stale cases, verify the session file was cleaned up
			if tt.name == "corrupted JSON file" || tt.name == "stale session with dead PID" {
				_, statErr := os.Stat(filepath.Join(tmpDir, "session.json"))
				assert.True(t, os.IsNotExist(statErr), "session file should be removed for %s", tt.name)
			}

			// For active session, verify the file is still present
			if tt.name == "active session with current PID" {
				_, statErr := os.Stat(filepath.Join(tmpDir, "session.json"))
				assert.NoError(t, statErr, "session file should still exist for active session")
			}
		})
	}
}

func mustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
