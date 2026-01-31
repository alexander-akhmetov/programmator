package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gitutil "github.com/worksonmyai/programmator/internal/git"
	"github.com/worksonmyai/programmator/internal/safety"
)

// wrapText wraps text to fit within width, with optional indent for continuation lines.
// maxLines limits output; 0 means unlimited. Truncates with "..." if exceeded.
func wrapText(text string, width int, indent string, maxLines int) string {
	if width <= 0 {
		return text
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	currentLine := words[0]
	firstLine := true
	contWidth := width - len(indent)

	for _, word := range words[1:] {
		lineWidth := width
		if !firstLine {
			lineWidth = contWidth
		}

		if len(currentLine)+1+len(word) <= lineWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			if maxLines > 0 && len(lines) >= maxLines {
				last := lines[len(lines)-1]
				if len(last) > 3 {
					lines[len(lines)-1] = last[:len(last)-3] + "..."
				}
				return strings.Join(lines, "\n")
			}
			firstLine = false
			currentLine = indent + word
		}
	}
	lines = append(lines, currentLine)

	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		last := lines[len(lines)-1]
		if len(last) > 3 {
			lines[len(lines)-1] = last[:len(last)-3] + "..."
		}
	}

	return strings.Join(lines, "\n")
}

func sectionHeader(title string, width int) string {
	padding := max(1, (width-len(title)-2)/2)
	line := strings.Repeat("─", padding)
	return labelStyle.Render(line+" ") + valueStyle.Render(title) + labelStyle.Render(" "+line)
}

func getGitInfo(workingDir string) (branch string, dirty bool) {
	repo, err := gitutil.NewRepo(workingDir)
	if err != nil {
		return "", false
	}
	branch, err = repo.CurrentBranch()
	if err != nil {
		return "", false
	}
	hasChanges, err := repo.HasUncommittedChanges()
	if err != nil {
		return branch, false
	}
	return branch, hasChanges
}

func abbreviatePath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) > 3 {
		return filepath.Join(parts[len(parts)-3:]...)
	}
	return path
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func formatTokens(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func formatMemory(kb int64) string {
	if kb >= 1024*1024 {
		return fmt.Sprintf("%.1fGB", float64(kb)/(1024*1024))
	}
	if kb >= 1024 {
		return fmt.Sprintf("%.0fMB", float64(kb)/1024)
	}
	return fmt.Sprintf("%dKB", kb)
}

func shortenModelName(model string) string {
	model = strings.TrimPrefix(model, "claude-")
	if idx := strings.LastIndex(model, "-20"); idx > 0 {
		model = model[:idx]
	}
	return model
}

// sortedModelNames returns model names from the token map in sorted order.
func sortedModelNames(tokensByModel map[string]*safety.ModelTokens) []string {
	models := make([]string, 0, len(tokensByModel))
	for model := range tokensByModel {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}
