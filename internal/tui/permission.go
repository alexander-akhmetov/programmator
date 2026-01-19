package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-akhmetov/programmator/internal/permission"
)

var (
	dialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(1, 2).
			Align(lipgloss.Center)

	dialogTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205"))

	toolNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("117"))

	toolInputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Width(60)

	optionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	selectedOptionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("42"))

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))
)

type PermissionDialog struct {
	request      *permission.Request
	selectedIdx  int
	responseChan chan<- permission.Decision
}

type dialogOption struct {
	key      string
	label    string
	decision permission.Decision
}

var dialogOptions = []dialogOption{
	{key: "a", label: "Allow once", decision: permission.DecisionAllow},
	{key: "p", label: "Allow for project", decision: permission.DecisionAllowProject},
	{key: "g", label: "Allow globally", decision: permission.DecisionAllowGlobal},
	{key: "d", label: "Deny", decision: permission.DecisionDeny},
}

func NewPermissionDialog(req *permission.Request, respChan chan<- permission.Decision) *PermissionDialog {
	return &PermissionDialog{
		request:      req,
		selectedIdx:  0,
		responseChan: respChan,
	}
}

func (d *PermissionDialog) HandleKey(key string) bool {
	switch key {
	case "up", "k":
		if d.selectedIdx > 0 {
			d.selectedIdx--
		}
		return true
	case "down", "j":
		if d.selectedIdx < len(dialogOptions)-1 {
			d.selectedIdx++
		}
		return true
	case "enter", " ":
		d.respond(dialogOptions[d.selectedIdx].decision)
		return true
	case "a":
		d.respond(permission.DecisionAllow)
		return true
	case "p":
		d.respond(permission.DecisionAllowProject)
		return true
	case "g":
		d.respond(permission.DecisionAllowGlobal)
		return true
	case "d", "n":
		d.respond(permission.DecisionDeny)
		return true
	}
	return false
}

func (d *PermissionDialog) respond(decision permission.Decision) {
	if d.responseChan != nil {
		d.responseChan <- decision
		d.responseChan = nil
	}
}

func (d *PermissionDialog) View(width, height int) string {
	var b strings.Builder

	b.WriteString(dialogTitleStyle.Render("🔐 PERMISSION REQUEST"))
	b.WriteString("\n\n")

	b.WriteString("Tool: ")
	b.WriteString(toolNameStyle.Render(d.request.ToolName))
	b.WriteString("\n\n")

	if d.request.Description != "" {
		inputDisplay := d.request.Description
		if len(inputDisplay) > 80 {
			inputDisplay = inputDisplay[:77] + "..."
		}
		b.WriteString(toolInputStyle.Render(inputDisplay))
		b.WriteString("\n\n")
	}

	b.WriteString("─────────────────────────────────\n\n")

	for i, opt := range dialogOptions {
		style := optionStyle
		marker := "  "
		if i == d.selectedIdx {
			style = selectedOptionStyle
			marker = "▶ "
		}

		fmt.Fprintf(&b, "%s%s %s\n",
			marker,
			keyStyle.Render("["+opt.key+"]"),
			style.Render(opt.label),
		)
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k to select • Enter to confirm • a/p/g/d for quick select"))

	content := b.String()
	dialogWidth := min(70, width-4)
	dialog := dialogBoxStyle.Width(dialogWidth).Render(content)

	padTop := max(0, (height-lipgloss.Height(dialog))/2)
	padLeft := max(0, (width-lipgloss.Width(dialog))/2)

	return strings.Repeat("\n", padTop) + strings.Repeat(" ", padLeft) + dialog
}
