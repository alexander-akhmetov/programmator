package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPrompts_Embedded(t *testing.T) {
	// Load prompts with empty dirs to use only embedded defaults
	prompts, err := LoadPrompts("", "")
	require.NoError(t, err)
	require.NotNil(t, prompts)

	// Check that all prompts are loaded and non-empty
	assert.NotEmpty(t, prompts.Phased, "phased prompt should be loaded")
	assert.NotEmpty(t, prompts.Phaseless, "phaseless prompt should be loaded")
	assert.NotEmpty(t, prompts.ReviewFirst, "review_first prompt should be loaded")
	assert.NotEmpty(t, prompts.PlanCreate, "plan_create prompt should be loaded")

	// Check that comment lines are stripped
	assert.NotContains(t, prompts.Phased, "# Phased execution prompt")
	assert.NotContains(t, prompts.Phaseless, "# Phaseless execution prompt")

	// Check that template variables are present
	assert.Contains(t, prompts.Phased, "{{.ID}}")
	assert.Contains(t, prompts.Phased, "{{.Title}}")
	assert.Contains(t, prompts.Phased, "{{.CurrentPhase}}")
	assert.Contains(t, prompts.Phaseless, "{{.ID}}")
	assert.Contains(t, prompts.ReviewFirst, "{{.BaseBranch}}")
}

func TestLoadPrompts_GlobalOverride(t *testing.T) {
	// Create temp global dir
	globalDir := t.TempDir()
	promptsDir := filepath.Join(globalDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0o755))

	// Create a custom phased prompt
	customPrompt := "Custom phased prompt with {{.ID}}"
	require.NoError(t, os.WriteFile(filepath.Join(promptsDir, "phased.md"), []byte(customPrompt), 0o644))

	// Load prompts
	prompts, err := LoadPrompts(globalDir, "")
	require.NoError(t, err)
	require.NotNil(t, prompts)

	// Custom prompt should be loaded
	assert.Equal(t, customPrompt, prompts.Phased)
	// Other prompts should fall back to embedded
	assert.Contains(t, prompts.Phaseless, "{{.ID}}")
}

func TestLoadPrompts_LocalOverridesGlobal(t *testing.T) {
	// Create temp dirs
	globalDir := t.TempDir()
	localDir := t.TempDir()
	globalPromptsDir := filepath.Join(globalDir, "prompts")
	localPromptsDir := filepath.Join(localDir, "prompts")
	require.NoError(t, os.MkdirAll(globalPromptsDir, 0o755))
	require.NoError(t, os.MkdirAll(localPromptsDir, 0o755))

	// Create global and local prompts
	globalPrompt := "Global phased prompt"
	localPrompt := "Local phased prompt"
	require.NoError(t, os.WriteFile(filepath.Join(globalPromptsDir, "phased.md"), []byte(globalPrompt), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(localPromptsDir, "phased.md"), []byte(localPrompt), 0o644))

	// Load prompts
	prompts, err := LoadPrompts(globalDir, localDir)
	require.NoError(t, err)
	require.NotNil(t, prompts)

	// Local should override global
	assert.Equal(t, localPrompt, prompts.Phased)
}

func TestLoadPrompts_NonexistentGlobalFallsToEmbedded(t *testing.T) {
	prompts, err := LoadPrompts("/nonexistent/global/dir", "")
	require.NoError(t, err)
	require.NotNil(t, prompts)

	assert.NotEmpty(t, prompts.Phased)
	assert.NotEmpty(t, prompts.Phaseless)
	assert.NotEmpty(t, prompts.ReviewFirst)
	assert.NotEmpty(t, prompts.PlanCreate)
}

func TestLoadPrompts_LocalPermissionErrorFallsBack(t *testing.T) {
	// Create a local dir with an unreadable prompt file
	localDir := t.TempDir()
	promptsDir := filepath.Join(localDir, "prompts")
	require.NoError(t, os.MkdirAll(promptsDir, 0o755))

	// Create an unreadable file
	unreadable := filepath.Join(promptsDir, "phased.md")
	require.NoError(t, os.WriteFile(unreadable, []byte("custom"), 0o644))
	require.NoError(t, os.Chmod(unreadable, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	// Should fall back to embedded instead of failing
	prompts, err := LoadPrompts("", localDir)
	require.NoError(t, err)
	require.NotNil(t, prompts)
	assert.NotEmpty(t, prompts.Phased, "should fall back to embedded prompt")
}

func TestStripComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single comment line",
			input:    "# comment\ncontent",
			expected: "content",
		},
		{
			name:     "comment with indentation",
			input:    "  # indented comment\ncontent",
			expected: "content",
		},
		{
			name:     "multiple comments",
			input:    "# first\n# second\ncontent",
			expected: "content",
		},
		{
			name:     "no comments",
			input:    "line1\nline2",
			expected: "line1\nline2",
		},
		{
			name:     "crlf line endings",
			input:    "# comment\r\ncontent",
			expected: "content",
		},
		{
			name:     "hash in middle of line preserved",
			input:    "content # not a comment",
			expected: "content # not a comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripComments(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}
