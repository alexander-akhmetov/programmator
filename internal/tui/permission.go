package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-akhmetov/programmator/internal/permission"
)

var (
	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(1, 2)

	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	toolNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117"))

	toolInputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	permLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	optionActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("42"))

	optionInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))
)

type scopeType int

const (
	scopeOnce scopeType = iota
	scopeSession
	scopeProject
	scopeGlobal
)

type PermissionDialog struct {
	request      *permission.Request
	responseChan chan<- permission.HandlerResponse

	allowOptions []allowOption
	allowIdx     int // which allow option is checked
	scope        scopeType

	cursor   int // global cursor position across all items
	repoRoot string
}

type allowOption struct {
	label   string
	pattern string
}

func NewPermissionDialog(req *permission.Request, respChan chan<- permission.HandlerResponse) *PermissionDialog {
	d := &PermissionDialog{
		request:      req,
		responseChan: respChan,
		scope:        scopeOnce, // Default to one-time allow
	}

	d.repoRoot = detectGitRoot(req.Description)
	d.allowOptions = d.buildAllowOptions()

	return d
}

func (d *PermissionDialog) buildAllowOptions() []allowOption {
	toolName := d.request.ToolName
	input := d.request.Description

	var options []allowOption

	switch toolName {
	case "Read", "Write", "Edit", "Glob", "Grep":
		// File-based tools
		options = append(options, allowOption{
			label:   "This path",
			pattern: fmt.Sprintf("%s(%s)", toolName, input),
		})

		if dir := filepath.Dir(input); dir != "" && dir != "." {
			options = append(options, allowOption{
				label:   fmt.Sprintf("Directory %s", abbreviatePath(dir)),
				pattern: fmt.Sprintf("%s(%s:*)", toolName, dir),
			})
		}

		if d.repoRoot != "" && strings.HasPrefix(input, d.repoRoot) {
			options = append(options, allowOption{
				label:   fmt.Sprintf("Entire repo %s", abbreviatePath(d.repoRoot)),
				pattern: fmt.Sprintf("%s(%s:*)", toolName, d.repoRoot),
			})
		}

		// Allow all for this tool
		options = append(options, allowOption{
			label:   fmt.Sprintf("All %s operations", toolName),
			pattern: toolName,
		})

	case "Bash":
		// Command-based
		options = append(options, allowOption{
			label:   "This exact command",
			pattern: fmt.Sprintf("Bash(%s)", input),
		})

		// Extract command parts and build prefix patterns
		parts := strings.Fields(input)

		// Find how many consecutive subcommands (non-flags, non-paths) we have
		subcommandDepth := 1                       // at least the base command
		for i := 1; i < len(parts) && i < 4; i++ { // limit to 4 levels
			p := parts[i]
			// Stop at flags, paths, or path-like arguments
			if strings.HasPrefix(p, "-") || strings.HasPrefix(p, "/") || strings.HasPrefix(p, ".") || strings.Contains(p, "/") {
				break
			}
			subcommandDepth++
		}

		// Add patterns from most specific to least specific
		// e.g., for "yarn portal test foo": yarn portal test, yarn portal, yarn
		for depth := subcommandDepth; depth >= 1; depth-- {
			prefix := strings.Join(parts[:depth], " ")
			if depth == 1 {
				options = append(options, allowOption{
					label:   fmt.Sprintf("All '%s' commands", prefix),
					pattern: fmt.Sprintf("Bash(%s:*)", prefix),
				})
			} else {
				options = append(options, allowOption{
					label:   fmt.Sprintf("'%s ...' commands", prefix),
					pattern: fmt.Sprintf("Bash(%s:*)", prefix),
				})
			}
		}

		// All Bash
		options = append(options, allowOption{
			label:   "All Bash commands",
			pattern: "Bash",
		})

	default:
		// Generic tool
		options = append(options, allowOption{
			label:   "This request",
			pattern: permission.FormatPattern(toolName, input),
		})
		options = append(options, allowOption{
			label:   fmt.Sprintf("All %s operations", toolName),
			pattern: toolName,
		})
	}

	return options
}

func (d *PermissionDialog) HandleKey(key string) bool {
	totalItems := len(d.allowOptions) + 4 // 4 scope options

	switch key {
	case "up", "k":
		if d.cursor > 0 {
			d.cursor--
		}
		return false
	case "down", "j":
		if d.cursor < totalItems-1 {
			d.cursor++
		}
		return false
	case " ":
		// Space selects the item under cursor
		if d.cursor < len(d.allowOptions) {
			d.allowIdx = d.cursor
		} else {
			d.scope = scopeType(d.cursor - len(d.allowOptions))
		}
		return false
	case "enter":
		d.respond()
		return true
	case "d", "n", "escape":
		d.respondDeny()
		return true
	}
	return false
}

func (d *PermissionDialog) respond() {
	if d.responseChan == nil {
		return
	}

	pattern := d.allowOptions[d.allowIdx].pattern

	var decision permission.Decision
	switch d.scope {
	case scopeOnce:
		decision = permission.DecisionAllowOnce
	case scopeSession:
		decision = permission.DecisionAllow
	case scopeProject:
		decision = permission.DecisionAllowProject
	case scopeGlobal:
		decision = permission.DecisionAllowGlobal
	}

	d.responseChan <- permission.HandlerResponse{
		Decision: decision,
		Pattern:  pattern,
	}
	d.responseChan = nil
}

func (d *PermissionDialog) respondDeny() {
	if d.responseChan != nil {
		d.responseChan <- permission.HandlerResponse{Decision: permission.DecisionDeny}
		d.responseChan = nil
	}
}

func (d *PermissionDialog) renderDialog(width int) string {
	var b strings.Builder

	b.WriteString(dialogTitleStyle.Render("🔐 PERMISSION REQUEST"))
	b.WriteString("\n\n")

	b.WriteString(permLabelStyle.Render("Tool: "))
	b.WriteString(toolNameStyle.Render(d.request.ToolName))
	b.WriteString("\n")

	if d.request.Description != "" {
		desc := d.request.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		b.WriteString(toolInputStyle.Render(desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n\n")

	// Allow options (vertical list)
	b.WriteString(permLabelStyle.Render("Allow:"))
	b.WriteString("\n")

	for i, opt := range d.allowOptions {
		label := abbreviateLabel(opt.label)
		checkbox := "[ ]"
		if i == d.allowIdx {
			checkbox = "[x]"
		}

		cursor := "  "
		style := optionInactiveStyle
		if d.cursor == i {
			cursor = "> "
			style = optionActiveStyle
		}
		b.WriteString(cursor)
		b.WriteString(style.Render(checkbox + " " + label))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Scope options (vertical list)
	b.WriteString(permLabelStyle.Render("Scope:"))
	b.WriteString("\n")

	scopes := []string{"Once", "Session", "Project", "Global"}
	for i, s := range scopes {
		checkbox := "[ ]"
		if scopeType(i) == d.scope {
			checkbox = "[x]"
		}

		cursorPos := len(d.allowOptions) + i
		cursor := "  "
		style := optionInactiveStyle
		if d.cursor == cursorPos {
			cursor = "> "
			style = optionActiveStyle
		}
		b.WriteString(cursor)
		b.WriteString(style.Render(checkbox + " " + s))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Show full command and pattern being approved
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n")

	// Full command
	cmd := d.request.Description
	if len(cmd) > 60 {
		cmd = cmd[:57] + "..."
	}
	b.WriteString(permLabelStyle.Render("Command: "))
	b.WriteString(toolInputStyle.Render(cmd))
	b.WriteString("\n")

	// Pattern being approved
	pattern := d.allowOptions[d.allowIdx].pattern
	if len(pattern) > 60 {
		pattern = pattern[:57] + "..."
	}
	b.WriteString(permLabelStyle.Render("Pattern: "))
	b.WriteString(toolInputStyle.Render(pattern))
	b.WriteString("\n\n")

	// Help
	b.WriteString(permLabelStyle.Render("↑/↓: Move • Space: Select • Enter: Confirm • d: Deny"))

	dialogWidth := min(70, width-4)
	return dialogBoxStyle.Width(dialogWidth).Render(b.String())
}

func (d *PermissionDialog) View(width, height int) string {
	dialog := d.renderDialog(width)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog)
}

func (d *PermissionDialog) ViewWithBackground(width, height int, _ string) string {
	dialog := d.renderDialog(width)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, dialog)
}

// GetSelectedPattern returns the currently selected permission pattern
func (d *PermissionDialog) GetSelectedPattern() string {
	if d.allowIdx < len(d.allowOptions) {
		return d.allowOptions[d.allowIdx].pattern
	}
	return ""
}

func abbreviateLabel(label string) string {
	var result string

	// Handle specific patterns
	switch {
	case label == "This exact command":
		result = "Exact"
	case label == "This path":
		result = "Path"
	case label == "This request":
		result = "This"
	case strings.HasPrefix(label, "All Bash"):
		result = "All Bash"
	case strings.HasPrefix(label, "All '"):
		// "All 'yarn' commands" -> "yarn:*"
		start := strings.Index(label, "'") + 1
		end := strings.LastIndex(label, "'")
		if start > 0 && end > start {
			result = label[start:end] + ":*"
		}
	case strings.HasPrefix(label, "'"):
		// "'yarn portal test path/to/file ...' commands" -> extract just command words
		end := strings.Index(label, " ...")
		if end > 1 {
			content := label[1:end] // remove leading '
			// Only keep command words (before any path-like content)
			parts := strings.Fields(content)
			var cmdParts []string
			for _, p := range parts {
				if strings.Contains(p, "/") || strings.HasPrefix(p, ".") || strings.HasPrefix(p, "-") {
					break
				}
				cmdParts = append(cmdParts, p)
			}
			if len(cmdParts) > 0 {
				result = strings.Join(cmdParts, " ") + ":*"
			} else {
				result = content + ":*"
			}
		}
	case strings.HasPrefix(label, "Directory"):
		result = "Dir"
	case strings.HasPrefix(label, "Entire repo"):
		result = "Repo"
	case strings.HasPrefix(label, "All "):
		// "All Read operations" -> "All Read"
		parts := strings.Fields(label)
		if len(parts) >= 2 {
			result = parts[0] + " " + parts[1]
		} else {
			result = "All"
		}
	default:
		result = label
	}

	// Truncate if still too long
	if len(result) > 18 {
		return result[:15] + "..."
	}
	return result
}

func detectGitRoot(path string) string {
	if path == "" {
		return ""
	}

	// Start from the path and go up
	dir := path
	if !filepath.IsAbs(dir) {
		return ""
	}

	// Check if it's a file, get directory
	dir = filepath.Dir(dir)

	for dir != "/" && dir != "." {
		cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
		dir = filepath.Dir(dir)
	}

	return ""
}
