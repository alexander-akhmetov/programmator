package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatPattern(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolInput string
		expected  string
	}{
		{
			name:      "tool only",
			toolName:  "Read",
			toolInput: "",
			expected:  "Read",
		},
		{
			name:      "bash with command",
			toolName:  "Bash",
			toolInput: "git status",
			expected:  "Bash(git status)",
		},
		{
			name:      "long input truncated",
			toolName:  "Bash",
			toolInput: string(make([]byte, 150)),
			expected:  "Bash(" + string(make([]byte, 100)) + ")",
		},
		{
			name:      "multiline input first line only",
			toolName:  "Bash",
			toolInput: "echo hello\necho world",
			expected:  "Bash(echo hello)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPattern(tt.toolName, tt.toolInput)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		target   string
		expected bool
	}{
		{
			name:     "exact match",
			pattern:  "Bash(git status)",
			target:   "Bash(git status)",
			expected: true,
		},
		{
			name:     "tool only matches any arg",
			pattern:  "Bash",
			target:   "Bash(git status)",
			expected: true,
		},
		{
			name:     "wildcard prefix match",
			pattern:  "Bash(git:*)",
			target:   "Bash(git status)",
			expected: true,
		},
		{
			name:     "wildcard prefix no match",
			pattern:  "Bash(git:*)",
			target:   "Bash(npm install)",
			expected: false,
		},
		{
			name:     "different tool no match",
			pattern:  "Read",
			target:   "Bash(ls)",
			expected: false,
		},
		{
			name:     "exact arg match",
			pattern:  "Bash(ls -la)",
			target:   "Bash(ls -la)",
			expected: true,
		},
		{
			name:     "exact arg no match",
			pattern:  "Bash(ls -la)",
			target:   "Bash(ls)",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchPattern(tt.pattern, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseToolPattern(t *testing.T) {
	tests := []struct {
		input        string
		expectedTool string
		expectedArg  string
	}{
		{"Bash", "Bash", ""},
		{"Bash(git status)", "Bash", "git status"},
		{"Read(/path/to/file)", "Read", "/path/to/file"},
		{"Bash(git:*)", "Bash", "git:*"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tool, arg := parseToolPattern(tt.input)
			assert.Equal(t, tt.expectedTool, tool)
			assert.Equal(t, tt.expectedArg, arg)
		})
	}
}

func TestSettingsLoadAllowList(t *testing.T) {
	tmpDir := t.TempDir()

	settingsPath := filepath.Join(tmpDir, "settings.json")
	settings := claudeSettings{
		Permissions: &permissionsBlock{
			Allow: []string{"Bash(git:*)", "Read"},
		},
	}
	data, _ := json.Marshal(settings)
	require.NoError(t, os.WriteFile(settingsPath, data, 0644))

	s := &Settings{globalPath: settingsPath}
	allowList := s.loadAllowList(settingsPath)

	assert.Equal(t, []string{"Bash(git:*)", "Read"}, allowList)
}

func TestSettingsLoadAllowListEmpty(t *testing.T) {
	s := &Settings{globalPath: "/nonexistent/path"}
	allowList := s.loadAllowList("/nonexistent/path")
	assert.Nil(t, allowList)
}

func TestSettingsAddPermission(t *testing.T) {
	tmpDir := t.TempDir()

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")

	s := &Settings{globalPath: settingsPath}
	err := s.AddPermission("Bash", "git status", ScopeGlobal)
	require.NoError(t, err)

	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings claudeSettings
	require.NoError(t, json.Unmarshal(data, &settings))

	assert.Contains(t, settings.Permissions.Allow, "Bash(git status)")
}

func TestSettingsAddPermissionDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	initial := claudeSettings{
		Permissions: &permissionsBlock{
			Allow: []string{"Bash(git status)"},
		},
	}
	data, _ := json.Marshal(initial)
	require.NoError(t, os.WriteFile(settingsPath, data, 0644))

	s := &Settings{globalPath: settingsPath}
	err := s.AddPermission("Bash", "git status", ScopeGlobal)
	require.NoError(t, err)

	data, err = os.ReadFile(settingsPath)
	require.NoError(t, err)

	var settings claudeSettings
	require.NoError(t, json.Unmarshal(data, &settings))

	count := 0
	for _, p := range settings.Permissions.Allow {
		if p == "Bash(git status)" {
			count++
		}
	}
	assert.Equal(t, 1, count, "should not add duplicate")
}

func TestSettingsIsAllowed(t *testing.T) {
	tmpDir := t.TempDir()

	globalPath := filepath.Join(tmpDir, "global", "settings.json")
	projectPath := filepath.Join(tmpDir, "project", "settings.local.json")

	require.NoError(t, os.MkdirAll(filepath.Dir(globalPath), 0755))
	require.NoError(t, os.MkdirAll(filepath.Dir(projectPath), 0755))

	globalSettings := claudeSettings{
		Permissions: &permissionsBlock{
			Allow: []string{"Bash(git:*)"},
		},
	}
	globalData, _ := json.Marshal(globalSettings)
	require.NoError(t, os.WriteFile(globalPath, globalData, 0644))

	projectSettings := claudeSettings{
		Permissions: &permissionsBlock{
			Allow: []string{"Read"},
		},
	}
	projectData, _ := json.Marshal(projectSettings)
	require.NoError(t, os.WriteFile(projectPath, projectData, 0644))

	s := &Settings{
		globalPath:  globalPath,
		projectPath: projectPath,
	}

	assert.True(t, s.IsAllowed("Bash", "git status"), "should match global wildcard")
	assert.True(t, s.IsAllowed("Read", "/any/file"), "should match project permission")
	assert.False(t, s.IsAllowed("Write", "/any/file"), "should not match")
}
