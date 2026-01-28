// Package plan provides parsing and management for markdown plan files.
// Plan files offer a lightweight alternative to tickets for driving programmator.
package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Task represents a single task within a plan.
type Task struct {
	Name      string
	Completed bool
}

// Plan represents a parsed plan file.
type Plan struct {
	// FilePath is the absolute path to the plan file.
	FilePath string
	// Title is extracted from the first # heading.
	Title string
	// ValidationCommands are commands to run after each task completion.
	ValidationCommands []string
	// Tasks are the checkboxed items in the plan.
	Tasks []Task
	// RawContent is the full file content.
	RawContent string
}

var (
	titleRegex      = regexp.MustCompile(`(?m)^#\s+(?:Plan:\s*)?(.+)$`)
	taskRegex       = regexp.MustCompile(`(?m)^-\s+\[([ xX])\]\s+(.+)$`)
	validationRegex = regexp.MustCompile("(?m)^-\\s+`([^`]+)`\\s*$")
)

// ParseFile reads and parses a plan file from disk.
func ParseFile(filePath string) (*Plan, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("read plan file: %w", err)
	}

	return Parse(absPath, string(content))
}

// Parse parses plan content from a string.
func Parse(filePath, content string) (*Plan, error) {
	plan := &Plan{
		FilePath:   filePath,
		RawContent: content,
	}

	// Extract title from first # heading
	if matches := titleRegex.FindStringSubmatch(content); len(matches) > 1 {
		plan.Title = strings.TrimSpace(matches[1])
	}

	// Parse validation commands from ## Validation Commands section
	plan.ValidationCommands = parseValidationCommands(content)

	// Parse tasks from checkboxes
	plan.Tasks = parseTasks(content)

	return plan, nil
}

// parseValidationCommands extracts validation commands from the plan.
// Commands are listed as `command` in the Validation Commands section.
func parseValidationCommands(content string) []string {
	// Find the Validation Commands section
	sectionStart := -1
	sectionEnd := len(content)
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Validation") || strings.HasPrefix(trimmed, "## Validation Commands") {
			sectionStart = i
			continue
		}
		// End section at next ## heading
		if sectionStart >= 0 && strings.HasPrefix(trimmed, "## ") {
			sectionEnd = i
			break
		}
	}

	if sectionStart < 0 {
		return nil
	}

	// Extract commands from section
	sectionContent := strings.Join(lines[sectionStart:min(sectionEnd, len(lines))], "\n")
	matches := validationRegex.FindAllStringSubmatch(sectionContent, -1)

	commands := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			commands = append(commands, match[1])
		}
	}

	return commands
}

// parseTasks extracts all checkbox tasks from the plan.
func parseTasks(content string) []Task {
	matches := taskRegex.FindAllStringSubmatch(content, -1)
	tasks := make([]Task, 0, len(matches))

	for _, match := range matches {
		if len(match) > 2 {
			tasks = append(tasks, Task{
				Name:      strings.TrimSpace(match[2]),
				Completed: match[1] != " ",
			})
		}
	}

	return tasks
}

// CurrentTask returns the first incomplete task, or nil if all are done.
func (p *Plan) CurrentTask() *Task {
	for i := range p.Tasks {
		if !p.Tasks[i].Completed {
			return &p.Tasks[i]
		}
	}
	return nil
}

// AllTasksComplete returns true if all tasks are completed.
func (p *Plan) AllTasksComplete() bool {
	for _, t := range p.Tasks {
		if !t.Completed {
			return false
		}
	}
	return len(p.Tasks) > 0
}

// MarkTaskComplete marks a task as completed by name.
// Returns an error if the task is not found or already completed.
func (p *Plan) MarkTaskComplete(taskName string) error {
	normalizedName := normalizeTaskName(taskName)

	// First pass: exact match
	for i := range p.Tasks {
		if !p.Tasks[i].Completed {
			existingName := normalizeTaskName(p.Tasks[i].Name)
			if existingName == normalizedName {
				p.Tasks[i].Completed = true
				return nil
			}
		}
	}

	// Second pass: existing task name contains the query (not vice versa)
	for i := range p.Tasks {
		if !p.Tasks[i].Completed {
			existingName := normalizeTaskName(p.Tasks[i].Name)
			if strings.Contains(existingName, normalizedName) {
				p.Tasks[i].Completed = true
				return nil
			}
		}
	}

	return fmt.Errorf("task not found or already completed: %s", taskName)
}

// normalizeTaskName strips common prefixes and normalizes for comparison.
func normalizeTaskName(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	// Remove common prefixes like "Task 1:", "Step 2:", "Phase 1:", etc.
	s = regexp.MustCompile(`^(task|step|phase)\s*\d+[:.]\s*`).ReplaceAllString(s, "")
	return s
}

// SaveFile writes the plan back to its file, updating checkbox states.
func (p *Plan) SaveFile() error {
	if p.FilePath == "" {
		return fmt.Errorf("plan has no file path")
	}

	content, err := os.ReadFile(p.FilePath)
	if err != nil {
		return fmt.Errorf("read plan file: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Track which task index we're matching
	taskIdx := 0

	for i, line := range lines {
		if match := taskRegex.FindStringSubmatch(line); match != nil {
			if taskIdx < len(p.Tasks) {
				task := p.Tasks[taskIdx]
				if task.Completed && match[1] == " " {
					lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
				}
				taskIdx++
			}
		}
	}

	return os.WriteFile(p.FilePath, []byte(strings.Join(lines, "\n")), 0644)
}

// ID returns the plan's identifier (base filename without extension).
func (p *Plan) ID() string {
	if p.FilePath == "" {
		return ""
	}
	base := filepath.Base(p.FilePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
