package progress

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/dirs"
)

// LogFile represents a progress log file.
type LogFile struct {
	Path      string
	SourceID  string
	Timestamp time.Time
	IsActive  bool // true if file is locked by an active session
}

// FindLogs finds log files in the logs directory, optionally filtered by source ID.
// Files are returned sorted by timestamp, newest first.
func FindLogs(logsDir, sourceID string) ([]LogFile, error) {
	if logsDir == "" {
		logsDir = dirs.LogsDir()
	}

	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No logs yet
		}
		return nil, err
	}

	var logs []LogFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		lf := parseLogFilename(logsDir, entry.Name())
		if lf == nil {
			continue
		}

		// Filter by source ID if specified
		if sourceID != "" && !strings.Contains(strings.ToLower(lf.SourceID), strings.ToLower(sourceID)) {
			continue
		}

		// Check if file is locked
		lf.IsActive = isFileLocked(lf.Path)

		logs = append(logs, *lf)
	}

	// Sort by timestamp, newest first
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp.After(logs[j].Timestamp)
	})

	return logs, nil
}

// FindLatestLog finds the most recent log file for a source ID.
func FindLatestLog(logsDir, sourceID string) (*LogFile, error) {
	logs, err := FindLogs(logsDir, sourceID)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

// FindActiveLog finds an active (locked) log file for a source ID.
func FindActiveLog(logsDir, sourceID string) (*LogFile, error) {
	logs, err := FindLogs(logsDir, sourceID)
	if err != nil {
		return nil, err
	}
	for i := range logs {
		if logs[i].IsActive {
			return &logs[i], nil
		}
	}
	return nil, nil
}

// parseLogFilename parses a log filename into a LogFile.
// Expected format: YYYYMMDD-HHMMSS-<source-id>.log
func parseLogFilename(dir, name string) *LogFile {
	// Remove .log suffix
	base := strings.TrimSuffix(name, ".log")

	// Need at least timestamp prefix: YYYYMMDD-HHMMSS (15 chars)
	if len(base) < 16 {
		return nil
	}

	// Parse timestamp
	tsStr := base[:15] // YYYYMMDD-HHMMSS
	t, err := time.Parse("20060102-150405", tsStr)
	if err != nil {
		return nil
	}

	// Extract source ID (everything after timestamp and dash)
	sourceID := ""
	if len(base) > 16 {
		sourceID = base[16:]
	}

	return &LogFile{
		Path:      filepath.Join(dir, name),
		SourceID:  sourceID,
		Timestamp: t,
	}
}

// isFileLocked checks if a file is locked by another process.
func isFileLocked(path string) bool {
	// First check if this process holds the lock
	if IsPathLockedByCurrentProcess(path) {
		return true
	}

	// Try to acquire a non-blocking lock
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	canLock, err := TryLockFile(f)
	if err != nil {
		return false
	}
	return !canLock // If we can't lock it, it's locked
}
