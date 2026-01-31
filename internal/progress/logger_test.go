package progress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/protocol"
)

func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewLogger(Config{
		LogsDir:    tmpDir,
		SourceID:   "test-ticket-123",
		SourceType: protocol.SourceTypeTicket,
		WorkDir:    "/tmp/test",
	})
	require.NoError(t, err)
	require.NotNil(t, logger)
	defer logger.Close()

	// Verify log file was created
	assert.FileExists(t, logger.Path())
	assert.Contains(t, logger.Path(), "test-ticket-123")
	assert.Equal(t, "test-ticket-123", logger.SourceID())

	// Write some logs
	logger.Printf("Test message %d", 1)
	logger.Iteration(1, 5, "Phase 1")
	logger.Status(protocol.StatusContinue.String(), "Working on it", []string{"file1.go", "file2.go"})
	logger.PhaseComplete("Phase 1")
	logger.Errorf("Test error: %s", "something went wrong")
	logger.ReviewStart(1, 3)
	logger.ReviewResult(false, 5)
	logger.Exit("complete", "", 3, []string{"file1.go"})

	// Close and read the file
	require.NoError(t, logger.Close())

	data, err := os.ReadFile(logger.Path())
	require.NoError(t, err)

	content := string(data)

	// Verify content
	assert.Contains(t, content, "# Programmator Progress Log")
	assert.Contains(t, content, "Source: test-ticket-123")
	assert.Contains(t, content, "Test message 1")
	assert.Contains(t, content, "Iteration 1/5")
	assert.Contains(t, content, "Phase: Phase 1")
	assert.Contains(t, content, "Status: CONTINUE")
	assert.Contains(t, content, "Files changed: file1.go, file2.go")
	assert.Contains(t, content, "Phase completed: Phase 1")
	assert.Contains(t, content, "ERROR: Test error:")
	assert.Contains(t, content, "Review 1/3")
	assert.Contains(t, content, "Review found 5 issues")
	assert.Contains(t, content, "Exit reason: complete")
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple-id", "simple-id"},
		{"./plans/my-plan.md", ".-plans-my-plan.md"},
		{"/path/to/file.md", "path-to-file.md"}, // leading dash trimmed
		{"has spaces here", "has-spaces-here"},
		{"has:colons:too", "has-colons-too"},
		{"special!@#$chars", "specialchars"},
		{"", "unnamed"},
		{"a", "a"},
		{strings.Repeat("a", 150), strings.Repeat("a", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindLogs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some test log files
	files := []string{
		"20260129-120000-ticket-1.log",
		"20260129-130000-ticket-2.log",
		"20260129-140000-plan-test.log",
	}
	for _, f := range files {
		path := filepath.Join(tmpDir, f)
		require.NoError(t, os.WriteFile(path, []byte("test"), 0644))
	}

	// Find all logs
	logs, err := FindLogs(tmpDir, "")
	require.NoError(t, err)
	assert.Len(t, logs, 3)

	// Should be sorted by timestamp, newest first
	assert.Equal(t, "plan-test", logs[0].SourceID)
	assert.Equal(t, "ticket-2", logs[1].SourceID)
	assert.Equal(t, "ticket-1", logs[2].SourceID)

	// Find logs with filter
	logs, err = FindLogs(tmpDir, "ticket")
	require.NoError(t, err)
	assert.Len(t, logs, 2)

	logs, err = FindLogs(tmpDir, "plan")
	require.NoError(t, err)
	assert.Len(t, logs, 1)
}

func TestFindLatestLog(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test log files
	path := filepath.Join(tmpDir, "20260129-120000-my-source.log")
	require.NoError(t, os.WriteFile(path, []byte("test"), 0644))

	lf, err := FindLatestLog(tmpDir, "my-source")
	require.NoError(t, err)
	require.NotNil(t, lf)
	assert.Equal(t, "my-source", lf.SourceID)
	assert.Equal(t, path, lf.Path)

	// Test with no matching logs
	lf, err = FindLatestLog(tmpDir, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, lf)
}

func TestParseLogFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		expectNil bool
		sourceID  string
	}{
		{"valid", "20260129-120000-my-source.log", false, "my-source"},
		{"valid no source", "20260129-120000-.log", false, ""},
		{"too short", "short.log", true, ""},
		{"invalid timestamp", "invalid-timestamp-source.log", true, ""},
		{"not a log file", "readme.md", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLogFilename("/tmp", tt.filename)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.sourceID, result.SourceID)
			}
		})
	}
}

func TestLockRegistry(t *testing.T) {
	path := "/tmp/test-lock-path.log"

	// Initially not locked
	assert.False(t, IsPathLockedByCurrentProcess(path))

	// Register lock
	registerActiveLock(path)
	assert.True(t, IsPathLockedByCurrentProcess(path))

	// Unregister lock
	unregisterActiveLock(path)
	assert.False(t, IsPathLockedByCurrentProcess(path))
}
