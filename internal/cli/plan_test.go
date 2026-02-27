package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expected    string
	}{
		{
			name:        "simple words",
			description: "Add user authentication",
			expected:    "add-user-authentication",
		},
		{
			name:        "special characters",
			description: "Fix bug #123: crash on login!",
			expected:    "fix-bug-123-crash-on-login",
		},
		{
			name:        "consecutive special chars collapsed",
			description: "hello   world",
			expected:    "hello-world",
		},
		{
			name:        "leading and trailing special chars",
			description: "  --hello world--  ",
			expected:    "hello-world",
		},
		{
			name:        "long description truncated",
			description: "This is a very long description that should be truncated to fifty characters maximum",
			expected:    "this-is-a-very-long-description-that-should-be-tru",
		},
		{
			name:        "truncation does not end with hyphen",
			description: "This is a very long description that should be tru-ncated cleanly",
			expected:    "this-is-a-very-long-description-that-should-be-tru",
		},
		{
			name:        "numbers preserved",
			description: "v2 migration step 3",
			expected:    "v2-migration-step-3",
		},
		{
			name:        "uppercase converted",
			description: "UPPERCASE Test",
			expected:    "uppercase-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateSlug(tt.description)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestLooksLikePlan(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "valid plan with title and tasks",
			output:   "# Plan: Add auth\n\n## Tasks\n- [ ] Implement login",
			expected: true,
		},
		{
			name:     "plan with space in title",
			output:   "# Plan Something\n\n- [ ] First task",
			expected: true,
		},
		{
			name:     "no title",
			output:   "## Tasks\n- [ ] First task",
			expected: false,
		},
		{
			name:     "no tasks",
			output:   "# Plan: Something\n\nJust a description",
			expected: false,
		},
		{
			name:     "empty string",
			output:   "",
			expected: false,
		},
		{
			name:     "unrelated content",
			output:   "Hello world, this is not a plan",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikePlan(tt.output)
			assert.Equal(t, tt.expected, got)
		})
	}
}
