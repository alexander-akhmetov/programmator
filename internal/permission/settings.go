// Package permission handles Claude Code permission management.
package permission

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Scope string

const (
	ScopeSession Scope = "session"
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

type Settings struct {
	globalPath  string
	projectPath string
	projectDir  string
}

type claudeSettings struct {
	Permissions *permissionsBlock `json:"permissions,omitempty"`
}

type permissionsBlock struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

func NewSettings(projectDir string) *Settings {
	home, _ := os.UserHomeDir()
	globalPath := filepath.Join(home, ".claude", "settings.json")

	projectPath := ""
	if projectDir != "" {
		projectPath = filepath.Join(projectDir, ".claude", "settings.local.json")
	}

	return &Settings{
		globalPath:  globalPath,
		projectPath: projectPath,
		projectDir:  projectDir,
	}
}

func (s *Settings) IsAllowed(toolName, toolInput string) bool {
	pattern := FormatPattern(toolName, toolInput)

	if s.matchesPatterns(s.loadAllowList(s.projectPath), pattern) {
		return true
	}
	if s.matchesPatterns(s.loadAllowList(s.globalPath), pattern) {
		return true
	}

	return false
}

func (s *Settings) AddPermission(toolName, toolInput string, scope Scope) error {
	pattern := FormatPattern(toolName, toolInput)

	var path string
	switch scope {
	case ScopeProject:
		if s.projectPath == "" {
			return fmt.Errorf("no project directory configured")
		}
		path = s.projectPath
	case ScopeGlobal:
		path = s.globalPath
	case ScopeSession:
		return fmt.Errorf("session scope cannot be persisted to file")
	}

	return s.addPatternToFile(path, pattern)
}

func (s *Settings) loadAllowList(path string) []string {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var settings claudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}

	if settings.Permissions == nil {
		return nil
	}

	return settings.Permissions.Allow
}

func (s *Settings) addPatternToFile(path, pattern string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	var settings claudeSettings

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse settings: %w", err)
		}
	}

	if settings.Permissions == nil {
		settings.Permissions = &permissionsBlock{}
	}

	if slices.Contains(settings.Permissions.Allow, pattern) {
		return nil
	}

	settings.Permissions.Allow = append(settings.Permissions.Allow, pattern)

	data, err = json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}

func (s *Settings) matchesPatterns(patterns []string, target string) bool {
	for _, pattern := range patterns {
		if MatchPattern(pattern, target) {
			return true
		}
	}
	return false
}

func FormatPattern(toolName, toolInput string) string {
	if toolInput == "" {
		return toolName
	}

	input := normalizeInput(toolInput)
	return fmt.Sprintf("%s(%s)", toolName, input)
}

func normalizeInput(input string) string {
	input = strings.TrimSpace(input)

	if len(input) > 100 {
		input = input[:100]
	}

	if strings.Contains(input, "\n") {
		lines := strings.SplitN(input, "\n", 2)
		input = lines[0]
	}

	return input
}

func MatchPattern(pattern, target string) bool {
	if pattern == target {
		return true
	}

	patternTool, patternArg := parseToolPattern(pattern)
	targetTool, targetArg := parseToolPattern(target)

	if patternTool != targetTool {
		return false
	}

	if patternArg == "" {
		return true
	}

	if prefix, found := strings.CutSuffix(patternArg, ":*"); found {
		return strings.HasPrefix(targetArg, prefix)
	}

	return patternArg == targetArg
}

func parseToolPattern(s string) (tool, arg string) {
	tool, arg, found := strings.Cut(s, "(")
	if !found {
		return s, ""
	}
	arg = strings.TrimSuffix(arg, ")")
	return tool, arg
}
