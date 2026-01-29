// Package progress provides persistent timestamped logging for all run modes.
// Every programmator execution writes a log file to ~/.programmator/logs/ with
// timestamped entries for iterations, phases, status updates, and exit reasons.
package progress

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// timestampFormat is the format for log timestamps.
const timestampFormat = "2006-01-02 15:04:05"

// Logger writes timestamped progress to a log file and optional io.Writer.
type Logger struct {
	file      *os.File
	writer    io.Writer // optional additional writer (e.g., for TUI)
	startTime time.Time
	sourceID  string
	workDir   string
	logPath   string
}

// Config holds logger configuration.
type Config struct {
	LogsDir    string    // Directory for log files (default: ~/.programmator/logs)
	SourceID   string    // Source identifier (ticket ID or plan filename)
	SourceType string    // "ticket" or "plan"
	WorkDir    string    // Working directory
	Writer     io.Writer // Optional additional writer for live output
}

// NewLogger creates a logger that writes to a timestamped log file.
// Log files are stored in LogsDir with format: <timestamp>-<source-id>.log
func NewLogger(cfg Config) (*Logger, error) {
	logsDir := cfg.LogsDir
	if logsDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		logsDir = filepath.Join(home, ".programmator", "logs")
	}

	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create logs dir: %w", err)
	}

	// Generate log filename: YYYYMMDD-HHMMSS-<source-id>.log
	timestamp := time.Now().Format("20060102-150405")
	sanitizedID := sanitizeFilename(cfg.SourceID)
	logPath := filepath.Join(logsDir, fmt.Sprintf("%s-%s.log", timestamp, sanitizedID))

	f, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	// Acquire exclusive lock on log file to signal active session
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}
	registerActiveLock(logPath)

	l := &Logger{
		file:      f,
		writer:    cfg.Writer,
		startTime: time.Now(),
		sourceID:  cfg.SourceID,
		workDir:   cfg.WorkDir,
		logPath:   logPath,
	}

	// Write header
	l.writef("# Programmator Progress Log\n")
	l.writef("Source: %s (%s)\n", cfg.SourceID, cfg.SourceType)
	l.writef("Working dir: %s\n", cfg.WorkDir)
	l.writef("Started: %s\n", time.Now().Format(timestampFormat))
	l.writef("%s\n\n", strings.Repeat("-", 60))

	return l, nil
}

// Path returns the log file path.
func (l *Logger) Path() string {
	return l.logPath
}

// SourceID returns the source identifier.
func (l *Logger) SourceID() string {
	return l.sourceID
}

// Printf writes a timestamped message to the log.
func (l *Logger) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format(timestampFormat)
	l.writef("[%s] %s\n", timestamp, msg)
}

// Section writes a section header to the log.
func (l *Logger) Section(title string) {
	l.writef("\n--- %s ---\n", title)
}

// Iteration logs the start of a new iteration.
func (l *Logger) Iteration(n, maxIter int, phase string) {
	l.Section(fmt.Sprintf("Iteration %d/%d", n, maxIter))
	if phase != "" {
		l.Printf("Phase: %s", phase)
	}
}

// Status logs a status update from Claude.
func (l *Logger) Status(status, summary string, filesChanged []string) {
	l.Printf("Status: %s", status)
	l.Printf("Summary: %s", summary)
	if len(filesChanged) > 0 {
		l.Printf("Files changed: %s", strings.Join(filesChanged, ", "))
	}
}

// PhaseComplete logs phase completion.
func (l *Logger) PhaseComplete(phase string) {
	l.Printf("Phase completed: %s", phase)
}

// Errorf logs an error message.
func (l *Logger) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format(timestampFormat)
	l.writef("[%s] ERROR: %s\n", timestamp, msg)
}

// ReviewStart logs the start of code review.
func (l *Logger) ReviewStart(iteration, maxIter int) {
	l.Section(fmt.Sprintf("Review %d/%d", iteration, maxIter))
}

// ReviewResult logs review results.
func (l *Logger) ReviewResult(passed bool, issueCount int) {
	if passed {
		l.Printf("Review PASSED")
	} else {
		l.Printf("Review found %d issues", issueCount)
	}
}

// Exit logs the exit reason and duration.
func (l *Logger) Exit(reason, message string, iterations int, filesChanged []string) {
	l.writef("\n%s\n", strings.Repeat("-", 60))
	l.writef("Exit reason: %s\n", reason)
	if message != "" {
		l.writef("Exit message: %s\n", message)
	}
	l.writef("Iterations: %d\n", iterations)
	if len(filesChanged) > 0 {
		l.writef("Files changed (%d): %s\n", len(filesChanged), strings.Join(filesChanged, ", "))
	}
	l.writef("Duration: %s\n", l.elapsed())
	l.writef("Completed: %s\n", time.Now().Format(timestampFormat))
}

// Close releases the file lock and closes the log file.
func (l *Logger) Close() error {
	if l.file == nil {
		return nil
	}

	_ = unlockFile(l.file)
	unregisterActiveLock(l.logPath)

	if err := l.file.Close(); err != nil {
		return fmt.Errorf("close log file: %w", err)
	}
	return nil
}

func (l *Logger) writef(format string, args ...any) {
	if l.file != nil {
		fmt.Fprintf(l.file, format, args...)
	}
}

func (l *Logger) elapsed() string {
	d := time.Since(l.startTime).Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// sanitizeFilename converts a source ID to a safe filename component.
func sanitizeFilename(s string) string {
	// Replace path separators and special chars with dashes
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, " ", "-")

	// Keep only alphanumeric, dashes, underscores, and dots
	var clean strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			clean.WriteRune(r)
		}
	}
	result := clean.String()

	// Collapse multiple dashes
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// Trim leading/trailing dashes
	result = strings.Trim(result, "-")

	// Limit length
	if len(result) > 100 {
		result = result[:100]
		result = strings.TrimRight(result, "-")
	}

	if result == "" {
		return "unnamed"
	}
	return result
}
