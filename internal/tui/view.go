package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	sidebarWidth := max(45, min(60, m.width*40/100))
	mainWidth := m.width - sidebarWidth - 4
	contentHeight := m.height - 3

	sidebar := m.renderSidebar(sidebarWidth-4, contentHeight-2)
	sidebarBox := statusBoxStyle.Width(sidebarWidth).Height(contentHeight).Render(sidebar)

	logHeader := "Logs"
	if m.logViewport.TotalLineCount() > 0 {
		logHeader = fmt.Sprintf("Logs (%d lines, %d%%)", m.logViewport.TotalLineCount(), int(m.logViewport.ScrollPercent()*100))
	}

	logsContent := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(logHeader) + "\n" + m.logViewport.View()

	logsBox := logBoxStyle.Width(mainWidth).Height(contentHeight).Render(logsContent)

	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, logsBox)
	return main + "\n" + m.renderHelp()
}

// renderSidebar composes all sidebar sections.
func (m Model) renderSidebar(width int, height int) string {
	var b strings.Builder

	// Render fixed sections first to calculate actual line usage
	header := m.renderSidebarHeader(width)
	ticket := m.renderSidebarTicket(width)
	environment := m.renderSidebarEnvironment(width)
	progress := m.renderSidebarProgress(width)
	usage := m.renderSidebarUsage(width)

	b.WriteString(header)
	b.WriteString(ticket)
	b.WriteString(environment)
	b.WriteString(progress)
	b.WriteString(usage)

	// Calculate actual lines used by fixed sections
	// Note: lipgloss.Height("") returns 1, so use heightOf to treat empty as 0
	usedLines := heightOf(header) + heightOf(ticket) +
		heightOf(environment) + heightOf(progress) + heightOf(usage)

	tips := m.renderSidebarTips(width)
	tipsLines := heightOf(tips)

	// Pre-render footer so its height can be reserved in the phases area
	footer := m.renderSidebarFooter()
	footerLines := heightOf(footer)

	// Reserve space for both tips and footer when rendering phases
	b.WriteString(m.renderSidebarPhases(width, height, usedLines, tipsLines+footerLines))
	b.WriteString(tips)
	b.WriteString(footer)

	return b.String()
}

func (m Model) renderSidebarHeader(width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ PROGRAMMATOR"))
	b.WriteString("\n")

	if strings.Contains(m.claudeFlags, "--dangerously-skip-permissions") {
		b.WriteString(dangerStyle.Render("⚠ SKIP PERMISSIONS"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	stateIndicator := m.getStateIndicator()

	if m.state != nil && !m.state.StartTime.IsZero() {
		elapsed := formatDuration(time.Since(m.state.StartTime))
		padding := width - lipgloss.Width(stateIndicator) - len(elapsed) - 2
		if padding > 0 {
			b.WriteString(stateIndicator)
			b.WriteString(strings.Repeat(" ", padding))
			b.WriteString(valueStyle.Render(elapsed))
		} else {
			b.WriteString(stateIndicator)
			b.WriteString("  ")
			b.WriteString(valueStyle.Render(elapsed))
		}
	} else {
		b.WriteString(stateIndicator)
	}
	b.WriteString("\n")

	if m.claudePID > 0 && m.runState == stateRunning {
		b.WriteString(labelStyle.Render(fmt.Sprintf("    pid %d • %s", m.claudePID, formatMemory(m.claudeMemKB))))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func (m Model) getStateIndicator() string {
	switch m.runState {
	case stateRunning:
		return runningStyle.Render(m.spinner.View() + " Running")
	case stateStopped:
		return stoppedStyle.Render("⏹ STOPPED")
	case stateComplete:
		return runningStyle.Render("✓ COMPLETE")
	default:
		return ""
	}
}

func (m Model) renderSidebarTicket(width int) string {
	var b strings.Builder

	b.WriteString(sectionHeader("Ticket", width))
	b.WriteString("\n")
	if m.workItem != nil {
		b.WriteString(valueStyle.Render(m.workItem.ID))
		b.WriteString("\n")
		wrappedTitle := wrapText(m.workItem.Title, width, "", 2)
		b.WriteString(labelStyle.Render(wrappedTitle))
		b.WriteString("\n")
	} else {
		b.WriteString(valueStyle.Render("-"))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderSidebarEnvironment(width int) string {
	if m.workingDir == "" && m.gitBranch == "" && m.claudeConfigDir == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sectionHeader("Environment", width))
	b.WriteString("\n")

	if m.workingDir != "" {
		b.WriteString(labelStyle.Render("Dir: "))
		b.WriteString(valueStyle.Render(abbreviatePath(m.workingDir)))
		b.WriteString("\n")
	}
	if m.gitBranch != "" {
		b.WriteString(labelStyle.Render("Git: "))
		branchStr := m.gitBranch
		if m.gitDirty {
			branchStr += " *"
		}
		b.WriteString(valueStyle.Render(branchStr))
		b.WriteString("\n")
	}
	if m.claudeConfigDir != "" {
		b.WriteString(labelStyle.Render("Claude: "))
		b.WriteString(valueStyle.Render(abbreviatePath(m.claudeConfigDir)))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderSidebarProgress(width int) string {
	if m.state == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sectionHeader("Progress", width))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Iteration: "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%d/%d", m.state.Iteration, m.config.MaxIterations)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Stagnation: "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%d/%d", m.state.ConsecutiveNoChanges, m.config.StagnationLimit)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Files: "))
	b.WriteString(valueStyle.Render(fmt.Sprintf("%d changed", len(m.state.TotalFilesChanged))))
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderSidebarUsage(width int) string {
	hasTokens := m.state != nil && (len(m.state.TokensByModel) > 0 || m.state.CurrentIterTokens != nil)
	if !hasTokens {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sectionHeader("Usage", width))
	b.WriteString("\n")

	models := sortedModelNames(m.state.TokensByModel)

	for _, model := range models {
		tokens := m.state.TokensByModel[model]
		shortModel := shortenModelName(model)
		b.WriteString(valueStyle.Render(shortModel))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("  "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("%s in / %s out", formatTokens(tokens.InputTokens), formatTokens(tokens.OutputTokens))))
		b.WriteString("\n")
	}

	if m.state.CurrentIterTokens != nil {
		b.WriteString(labelStyle.Render("current: "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("%s in / %s out",
			formatTokens(m.state.CurrentIterTokens.InputTokens),
			formatTokens(m.state.CurrentIterTokens.OutputTokens))))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderSidebarPhases(width int, height int, usedLines int, tipsLines int) string {
	if m.workItem == nil || len(m.workItem.Phases) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sectionHeader("Phases", width))
	b.WriteString("\n")
	b.WriteString(m.renderPhasesContent(width, height, usedLines, tipsLines))

	return b.String()
}

var sidebarTips = []string{
	"Did you know? `plan create` lets you build plans interactively",
	"Did you know? You can run plan files directly with `start ./plan.md`",
	"Did you know? `--auto-commit` commits after each completed phase",
	"Did you know? `run` lets you run Claude with a custom prompt",
	"Did you know? `config show` displays your resolved configuration",
	"Did you know? You can override prompt templates in `.programmator/prompts/`",
}

func (m Model) renderSidebarTips(width int) string {
	if m.hideTips {
		return ""
	}

	var b strings.Builder

	tip := sidebarTips[m.tipIndex%len(sidebarTips)]

	b.WriteString("\n")
	b.WriteString(sectionHeader("Tips", width))
	b.WriteString("\n")
	b.WriteString(tipStyle.Render(wrapText(tip, width, "", 2)))
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderSidebarFooter() string {
	var b strings.Builder

	if m.result != nil && m.runState == stateComplete {
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Exit: "))
		b.WriteString(valueStyle.Render(string(m.result.ExitReason)))
		if m.result.ExitMessage != "" {
			b.WriteString("\n")
			b.WriteString(labelStyle.Render("Reason: "))
			b.WriteString(valueStyle.Render(m.result.ExitMessage))
		}
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("Press q to quit"))
	}

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(stoppedStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return b.String()
}

func (m Model) renderPhasesContent(width int, height int, usedLines int, tipsLines int) string {
	var b strings.Builder

	currentPhase := m.workItem.CurrentPhase()
	currentIdx := -1
	for i, phase := range m.workItem.Phases {
		if currentPhase != nil && phase.Name == currentPhase.Name {
			currentIdx = i
			break
		}
	}

	// Account for the "Phases" header (3 lines: newline + header + newline) and tips/footer
	// Also subtract 1 for trailing newline effect on lipgloss.Height
	totalUsed := usedLines + 3 + tipsLines + 1
	availableForPhases := max(3, height-totalUsed)

	// Reserve space for "Done" block first (2 lines if complete)
	reservedLines := 0
	if m.runState == stateComplete {
		reservedLines += 2 // newline + "✓ Done"
	}

	// Preliminary space for phase lines after reserving for "Done"
	spaceForPhaseLines := max(1, availableForPhases-reservedLines)

	// If phases overflow the preliminary space, reserve lines for ↑ and ↓ indicators
	if len(m.workItem.Phases) > spaceForPhaseLines {
		reservedLines += 2 // ↑ and ↓ indicators
		spaceForPhaseLines = max(1, availableForPhases-reservedLines)
	}

	contextSize := max(0, (spaceForPhaseLines-1)/2)

	showFrom := 0
	showTo := len(m.workItem.Phases) - 1

	if len(m.workItem.Phases) > spaceForPhaseLines {
		if currentIdx >= 0 {
			showFrom = max(0, currentIdx-contextSize)
			showTo = min(len(m.workItem.Phases)-1, currentIdx+contextSize)
			// Ensure we show at most spaceForPhaseLines while including current phase
			if showTo-showFrom+1 > spaceForPhaseLines {
				showTo = showFrom + spaceForPhaseLines - 1
			}
		} else {
			// No current phase (e.g., all phases complete): show the last spaceForPhaseLines phases
			showTo = len(m.workItem.Phases) - 1
			showFrom = max(0, len(m.workItem.Phases)-spaceForPhaseLines)
		}
	}

	if showFrom > 0 {
		b.WriteString(labelStyle.Render(fmt.Sprintf("  ↑ %d more\n", showFrom)))
	}

	phaseWidth := width - 4
	for i := showFrom; i <= showTo; i++ {
		phase := m.workItem.Phases[i]
		wrappedName := wrapText(phase.Name, phaseWidth, "    ", 2)
		if phase.Completed {
			b.WriteString(runningStyle.Render("  ✓ "))
			b.WriteString(labelStyle.Render(wrappedName))
		} else if currentPhase != nil && phase.Name == currentPhase.Name {
			b.WriteString(phaseStyle.Render("  → "))
			b.WriteString(phaseStyle.Render(wrappedName))
		} else {
			b.WriteString(labelStyle.Render("  ○ "))
			b.WriteString(labelStyle.Render(wrappedName))
		}
		b.WriteString("\n")
	}

	if showTo < len(m.workItem.Phases)-1 {
		b.WriteString(labelStyle.Render(fmt.Sprintf("  ↓ %d more\n", len(m.workItem.Phases)-1-showTo)))
	}

	if m.runState == stateComplete {
		b.WriteString("\n")
		b.WriteString(runningStyle.Render("  ✓ Done"))
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderHelp() string {
	var parts []string

	if m.runState == stateRunning {
		parts = append(parts, "s: stop")
	}

	parts = append(parts, "↑/↓: scroll", "q: quit")

	return helpStyle.Render(strings.Join(parts, " • "))
}

// heightOf returns the rendered height of a string, treating empty strings as 0 lines.
// This is needed because lipgloss.Height("") returns 1.
func heightOf(s string) int {
	if s == "" {
		return 0
	}
	return lipgloss.Height(s)
}
