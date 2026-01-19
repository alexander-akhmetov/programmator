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

type focusField int

const (
	focusAllow focusField = iota
	focusScope
)

type PermissionDialog struct {
	request      *permission.Request
	responseChan chan<- permission.HandlerResponse

	allowOptions []allowOption
	allowIdx     int
	scope        scopeType
	focus        focusField // which row is focused

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

		// Extract command parts and build prefix patterns
		parts := strings.Fields(input)

		// Find how many consecutive subcommands (non-flags) we have
		subcommandDepth := 1                       // at least the base command
		for i := 1; i < len(parts) && i < 4; i++ { // limit to 4 levels
			if strings.HasPrefix(parts[i], "-") || strings.HasPrefix(parts[i], "/") || strings.HasPrefix(parts[i], ".") {
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
	switch key {
	case "tab":
		// Cycle focus forward
		d.focus = (d.focus + 1) % 2
		return false
	case "shift+tab", "backtab":
		// Cycle focus backward
		if d.focus == 0 {
			d.focus = 1
		} else {
			d.focus--
		}
		return false
	case "up", "k":
		// Move focus up
		if d.focus > 0 {
			d.focus--
		}
		return false
	case "down", "j":
		// Move focus down
		if d.focus < 1 {
			d.focus++
		}
		return false
	case "left", "h":
		// Navigate within focused field
		if d.focus == focusAllow {
			if d.allowIdx > 0 {
				d.allowIdx--
			}
		} else {
			if d.scope > 0 {
				d.scope--
			}
		}
		return false
	case "right", "l":
		// Navigate within focused field
		if d.focus == focusAllow {
			if d.allowIdx < len(d.allowOptions)-1 {
				d.allowIdx++
			}
		} else {
			if d.scope < 3 {
				d.scope++
			}
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

	// Allow options (horizontal)
	labelStyle := permLabelStyle
	if d.focus == focusAllow {
		labelStyle = optionActiveStyle
	}
	b.WriteString(labelStyle.Render("Allow: "))
	b.WriteString(keyHintStyle.Render("(←/→)"))
	b.WriteString("\n")

	for i, opt := range d.allowOptions {
		style := optionInactiveStyle
		label := abbreviateLabel(opt.label)
		if i == d.allowIdx {
			if d.focus == focusAllow {
				style = optionActiveStyle
				b.WriteString(style.Render("▶ " + label))
			} else {
				b.WriteString(style.Render("● " + label))
			}
		} else {
			b.WriteString(style.Render("  " + label))
		}
		b.WriteString("  ")
	}
	b.WriteString("\n\n")

	// Scope selector (horizontal)
	labelStyle = permLabelStyle
	if d.focus == focusScope {
		labelStyle = optionActiveStyle
	}
	b.WriteString(labelStyle.Render("Scope: "))
	b.WriteString(keyHintStyle.Render("(←/→)"))
	b.WriteString("  ")

	scopes := []string{"Once", "Session", "Project", "Global"}
	for i, s := range scopes {
		style := optionInactiveStyle
		if scopeType(i) == d.scope {
			if d.focus == focusScope {
				style = optionActiveStyle
				b.WriteString(style.Render("▶ " + s))
			} else {
				b.WriteString(style.Render("● " + s))
			}
		} else {
			b.WriteString(style.Render("  " + s))
		}
		b.WriteString("  ")
	}
	b.WriteString("\n\n")

	// Help
	b.WriteString(permLabelStyle.Render("Tab: Switch • ←/→: Select • Enter: Confirm • d: Deny"))

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
	// Shorten long labels for horizontal display
	if len(label) <= 20 {
		return label
	}

	// Handle specific patterns
	switch {
	case label == "This exact command":
		return "Exact"
	case label == "This path":
		return "Path"
	case label == "This request":
		return "This"
	case strings.HasPrefix(label, "All Bash"):
		return "All Bash"
	case strings.HasPrefix(label, "All '"):
		// "All 'yarn' commands" -> "yarn:*"
		start := strings.Index(label, "'") + 1
		end := strings.LastIndex(label, "'")
		if start > 0 && end > start {
			return label[start:end] + ":*"
		}
	case strings.HasPrefix(label, "'"):
		// "'yarn portal ...' commands" -> "yarn portal:*"
		end := strings.Index(label, " ...")
		if end > 1 {
			return label[1:end] + ":*"
		}
	case strings.HasPrefix(label, "Directory"):
		return "Dir"
	case strings.HasPrefix(label, "Entire repo"):
		return "Repo"
	case strings.HasPrefix(label, "All "):
		// "All Read operations" -> "All"
		return "All"
	}

	// Fallback: truncate
	if len(label) > 18 {
		return label[:15] + "..."
	}
	return label
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
