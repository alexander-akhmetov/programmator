package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	statusBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	logBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	phaseStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	progPrefixStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e67e22")).
			Bold(true)

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	toolResStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	diffDelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	diffHunkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))

	diffCtxStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	reviewStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))

	tipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Faint(true)
)
