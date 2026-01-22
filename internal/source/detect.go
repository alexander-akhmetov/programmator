package source

import (
	"os"
	"path/filepath"
	"strings"
)

// Detect determines the appropriate Source for the given identifier.
// It returns a TicketSource if the id is a ticket ID, or a PlanSource if it's a file path.
//
// Detection logic:
//   - If id looks like a file path (contains "/" or "\" or ends with ".md"), treat as plan
//   - If id exists as a file, treat as plan
//   - Otherwise, treat as ticket
func Detect(id string) (Source, string) {
	// Check if it looks like a file path
	if looksLikeFilePath(id) {
		return NewPlanSource(id), id
	}

	// Check if the id exists as a file
	if _, err := os.Stat(id); err == nil {
		absPath, _ := filepath.Abs(id)
		return NewPlanSource(absPath), absPath
	}

	// Default to ticket
	return NewTicketSource(nil), id
}

// looksLikeFilePath returns true if the string appears to be a file path.
func looksLikeFilePath(s string) bool {
	// Contains path separators
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return true
	}

	// Ends with .md
	if strings.HasSuffix(strings.ToLower(s), ".md") {
		return true
	}

	// Starts with . (relative path)
	if strings.HasPrefix(s, ".") {
		return true
	}

	return false
}

// IsPlanPath returns true if the path is a plan file path.
func IsPlanPath(path string) bool {
	return looksLikeFilePath(path) || fileExists(path)
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
