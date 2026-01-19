package cmd

import (
	"testing"
)

func TestExtractNotesSection(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "no notes section",
			content:  "# Ticket\n\nSome content",
			expected: "",
		},
		{
			name:     "notes section at end",
			content:  "# Ticket\n\n## Notes\n\nNote 1\nNote 2",
			expected: "## Notes\n\nNote 1\nNote 2",
		},
		{
			name: "notes section in middle",
			content: `# Ticket

## Notes

progress: something

## Acceptance`,
			expected: "## Notes\n\nprogress: something\n",
		},
		{
			name:     "empty notes section",
			content:  "# Ticket\n\n## Notes\n\n## Other",
			expected: "## Notes\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractNotesSection(tt.content)
			if got != tt.expected {
				t.Errorf("extractNotesSection() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestIsProgrammatorLogLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "progress prefix",
			line:     "progress: completed phase 1",
			expected: true,
		},
		{
			name:     "error prefix",
			line:     "error: something went wrong",
			expected: true,
		},
		{
			name:     "iter prefix",
			line:     "[iter 5] Did some work",
			expected: true,
		},
		{
			name:     "uppercase PROGRESS",
			line:     "PROGRESS: uppercase",
			expected: true,
		},
		{
			name:     "regular note",
			line:     "decision: using Go instead",
			expected: false,
		},
		{
			name:     "empty line",
			line:     "",
			expected: false,
		},
		{
			name:     "random text",
			line:     "Some other note",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProgrammatorLogLine(tt.line)
			if got != tt.expected {
				t.Errorf("isProgrammatorLogLine(%q) = %v, want %v", tt.line, got, tt.expected)
			}
		})
	}
}

func TestFormatLogLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "simple line",
			line:     "progress: did something",
			expected: "progress: did something",
		},
		{
			name:     "line with timestamp format",
			line:     "**2024-01-15T10:30:00Z** progress: did something",
			expected: "[2024-01-15T10:30:00Z] progress: did something",
		},
		{
			name:     "line without proper timestamp",
			line:     "**test** something",
			expected: "[test] something",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLogLine(tt.line)
			if got != tt.expected {
				t.Errorf("formatLogLine(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}
