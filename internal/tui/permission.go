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

	keyHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)
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
	allowIdx     int
	scope        scopeType

	repoRoot string // detected git repo root
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

		// Extract command prefix
		parts := strings.Fields(input)
		if len(parts) >= 2 && !strings.HasPrefix(parts[1], "-") {
			// Second word is a subcommand (not a flag), e.g., "yarn test", "go build"
			cmdWithSub := parts[0] + " " + parts[1]
			options = append(options, allowOption{
				label:   fmt.Sprintf("'%s ...' commands", cmdWithSub),
				pattern: fmt.Sprintf("Bash(%s:*)", cmdWithSub),
			})
		}
		if len(parts) >= 1 {
			// Also offer just the base command
			cmd := parts[0]
			options = append(options, allowOption{
				label:   fmt.Sprintf("All '%s' commands", cmd),
				pattern: fmt.Sprintf("Bash(%s:*)", cmd),
			})
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
	switch key {
	case "up", "k":
		if d.allowIdx > 0 {
			d.allowIdx--
		}
		return false
	case "down", "j":
		if d.allowIdx < len(d.allowOptions)-1 {
			d.allowIdx++
		}
		return false
	case "tab", "right":
		// Cycle forward through scopes (Once, Session, Project, Global)
		d.scope = (d.scope + 1) % 4
		return false
	case "left":
		// Cycle backward through scopes
		if d.scope == 0 {
			d.scope = 3
		} else {
			d.scope--
		}
		return false
	case "enter", " ":
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

	// Allow options
	b.WriteString(permLabelStyle.Render("Allow: "))
	b.WriteString(keyHintStyle.Render("(↑/↓)"))
	b.WriteString("\n")
	for i, opt := range d.allowOptions {
		style := optionInactiveStyle
		marker := "  "
		if i == d.allowIdx {
			style = optionActiveStyle
			marker = "▶ "
		}
		b.WriteString(marker)
		b.WriteString(style.Render(opt.label))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Scope selector
	b.WriteString(permLabelStyle.Render("Scope: "))
	b.WriteString(keyHintStyle.Render("(Tab)"))
	b.WriteString("  ")

	scopes := []string{"Once", "Session", "Project", "Global"}
	for i, s := range scopes {
		style := optionInactiveStyle
		if scopeType(i) == d.scope {
			style = optionActiveStyle
			b.WriteString(style.Render("▶ " + s))
		} else {
			b.WriteString(style.Render("  " + s))
		}
		if i < len(scopes)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n\n")

	// Help
	b.WriteString(permLabelStyle.Render("Enter: Confirm • d: Deny"))

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
