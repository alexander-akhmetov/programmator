package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "minutes and seconds",
			duration: 3*time.Minute + 25*time.Second,
			expected: "3m25s",
		},
		{
			name:     "hours minutes seconds",
			duration: 2*time.Hour + 15*time.Minute + 30*time.Second,
			expected: "2h15m30s",
		},
		{
			name:     "hours only (no minutes or seconds)",
			duration: 1 * time.Hour,
			expected: "1h0m0s",
		},
		{
			name:     "minutes only (no seconds)",
			duration: 5 * time.Minute,
			expected: "5m0s",
		},
		{
			name:     "rounds to nearest second",
			duration: 2*time.Minute + 30*time.Second + 500*time.Millisecond,
			expected: "2m31s",
		},
		{
			name:     "rounds down",
			duration: 2*time.Minute + 30*time.Second + 400*time.Millisecond,
			expected: "2m30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteSessionFile(t *testing.T) {
	tmpDir := t.TempDir()

	origEnv := os.Getenv("PROGRAMMATOR_STATE_DIR")
	defer os.Setenv("PROGRAMMATOR_STATE_DIR", origEnv)
	os.Setenv("PROGRAMMATOR_STATE_DIR", tmpDir)

	err := writeSessionFile("test-ticket-123", "/work/dir")
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(tmpDir, "session.json"))
	require.NoError(t, err)

	var session map[string]any
	err = json.Unmarshal(content, &session)
	require.NoError(t, err)

	require.Equal(t, "test-ticket-123", session["ticket_id"])
	require.Equal(t, "/work/dir", session["working_dir"])
	require.NotNil(t, session["started_at"])
	require.NotNil(t, session["pid"])
}

func TestRemoveSessionFile(t *testing.T) {
	tmpDir := t.TempDir()

	origEnv := os.Getenv("PROGRAMMATOR_STATE_DIR")
	defer os.Setenv("PROGRAMMATOR_STATE_DIR", origEnv)
	os.Setenv("PROGRAMMATOR_STATE_DIR", tmpDir)

	err := writeSessionFile("test-ticket", "/work")
	require.NoError(t, err)

	sessionPath := filepath.Join(tmpDir, "session.json")
	_, err = os.Stat(sessionPath)
	require.NoError(t, err, "session file should exist before removal")

	removeSessionFile()

	_, err = os.Stat(sessionPath)
	require.True(t, os.IsNotExist(err), "session file should be removed")
}

func TestRemoveSessionFileNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	origEnv := os.Getenv("PROGRAMMATOR_STATE_DIR")
	defer os.Setenv("PROGRAMMATOR_STATE_DIR", origEnv)
	os.Setenv("PROGRAMMATOR_STATE_DIR", tmpDir)

	removeSessionFile()
}
