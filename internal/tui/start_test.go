package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

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
