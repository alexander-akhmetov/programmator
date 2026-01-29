// Package tui implements the terminal user interface using bubbletea.
package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/worksonmyai/programmator/internal/debug"
	"github.com/worksonmyai/programmator/internal/loop"
	"github.com/worksonmyai/programmator/internal/permission"
	"github.com/worksonmyai/programmator/internal/progress"
	"github.com/worksonmyai/programmator/internal/review"
	"github.com/worksonmyai/programmator/internal/safety"
	"github.com/worksonmyai/programmator/internal/source"
	"github.com/worksonmyai/programmator/internal/timing"
)

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

	pausedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	dangerStyle = lipgloss.NewStyle().
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

	guardStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)
)

type runState int

const (
	stateRunning runState = iota
	statePaused
	stateStopped
	stateComplete
)

type Model struct {
	workItem         *source.WorkItem
	state            *safety.State
	config           safety.Config
	filesChanged     []string
	logs             []string
	logViewport      viewport.Model
	spinner          spinner.Model
	width            int
	height           int
	runState         runState
	loop             *loop.Loop
	ready            bool
	result           *loop.Result
	err              error
	renderer         *glamour.TermRenderer
	workingDir       string
	gitBranch        string
	gitDirty         bool
	claudePID        int
	claudeMemKB      int64
	guardMode        bool
	permissionDialog *PermissionDialog
}

func NewModel(config safety.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		state:    safety.NewState(),
		config:   config,
		logs:     make([]string, 0),
		spinner:  s,
		runState: stateRunning,
	}
}

type TicketUpdateMsg struct {
	WorkItem     *source.WorkItem
	State        *safety.State
	FilesChanged []string
}

type LogMsg struct {
	Text string
}

type LoopDoneMsg struct {
	Result *loop.Result
	Err    error
}

type ProcessStatsMsg struct {
	PID      int
	MemoryKB int64
}

type rendererReadyMsg struct {
	renderer *glamour.TermRenderer
}

type PermissionRequestMsg struct {
	Request      *permission.Request
	ResponseChan chan<- permission.HandlerResponse
}

func createRendererCmd(width int) tea.Cmd {
	return func() tea.Msg {
		viewportWidth := max(width-6, 40)
		renderer, _ := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(viewportWidth),
		)
		return rendererReadyMsg{renderer: renderer}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.WindowSize())
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.permissionDialog != nil {
		if m.permissionDialog.HandleKey(msg.String()) {
			m.permissionDialog = nil
		}
		return m, nil
	}

	var cmds []tea.Cmd

	switch msg.String() {
	case "q", "ctrl+c":
		if m.loop != nil && m.runState == stateRunning {
			m.loop.Stop()
		}
		return m, tea.Quit

	case "p":
		if m.loop != nil && (m.runState == stateRunning || m.runState == statePaused) {
			paused := m.loop.TogglePause()
			if paused {
				m.runState = statePaused
			} else {
				m.runState = stateRunning
			}
		}

	case "s":
		if m.loop != nil && m.runState != stateStopped && m.runState != stateComplete {
			m.loop.Stop()
			m.runState = stateStopped
		}

	case "up", "k", "down", "j", "pgup", "ctrl+u", "pgdown", "ctrl+d":
		var cmd tea.Cmd
		m.logViewport, cmd = m.logViewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case PermissionRequestMsg:
		debug.Logf("tui: received permission request for tool=%s", msg.Request.ToolName)
		m.permissionDialog = NewPermissionDialog(msg.Request, msg.ResponseChan)
		return m, nil

	case tea.WindowSizeMsg:
		timing.Log("Update: WindowSizeMsg received")
		m.width = msg.Width
		m.height = msg.Height

		// Calculate dimensions matching View()
		sidebarWidth := max(45, min(60, m.width*40/100))
		mainWidth := m.width - sidebarWidth - 4
		contentHeight := m.height - 3

		// Viewport dimensions (account for header line and box padding)
		viewportWidth := mainWidth - 4
		logHeight := contentHeight - 4

		if !m.ready {
			m.logViewport = viewport.New(viewportWidth, logHeight)
			m.logViewport.SetContent(m.wrapLogs())
			m.ready = true
			cmds = append(cmds, createRendererCmd(mainWidth))
		} else {
			m.logViewport.Width = viewportWidth
			m.logViewport.Height = logHeight
			m.logViewport.SetContent(m.wrapLogs())
		}

	case rendererReadyMsg:
		timing.Log("Update: rendererReadyMsg received")
		m.renderer = msg.renderer
		m.logViewport.SetContent(m.wrapLogs())

	case TicketUpdateMsg:
		m.workItem = msg.WorkItem
		m.state = msg.State
		m.filesChanged = msg.FilesChanged

	case LogMsg:
		m.logs = append(m.logs, msg.Text)
		if len(m.logs) > 5000 {
			m.logs = m.logs[len(m.logs)-5000:]
		}
		m.logViewport.SetContent(m.wrapLogs())
		m.logViewport.GotoBottom()

	case ProcessStatsMsg:
		m.claudePID = msg.PID
		m.claudeMemKB = msg.MemoryKB

	case LoopDoneMsg:
		m.result = msg.Result
		m.err = msg.Err
		if m.runState != stateStopped {
			m.runState = stateComplete
		}
		if msg.Result != nil {
			exitLine := fmt.Sprintf("\n[PROG]Loop finished: %s", msg.Result.ExitReason)
			if msg.Result.ExitMessage != "" {
				exitLine += fmt.Sprintf(" (%s)", msg.Result.ExitMessage)
			}
			exitLine += "\n"
			m.logs = append(m.logs, exitLine)
			m.logViewport.SetContent(m.wrapLogs())
			m.logViewport.GotoBottom()
		}
		if msg.Err != nil {
			m.logs = append(m.logs, fmt.Sprintf("\n[PROG]Loop error: %v\n", msg.Err))
			m.logViewport.SetContent(m.wrapLogs())
			m.logViewport.GotoBottom()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 {
		return "Initializing..."
	}

	// Sidebar width: 45-60 chars or 40% of screen
	sidebarWidth := max(45, min(60, m.width*40/100))

	// Main content area width
	mainWidth := m.width - sidebarWidth - 4 // 4 for borders/padding

	// Height for content (leave room for help line)
	contentHeight := m.height - 3

	// Build sidebar
	sidebar := m.renderSidebar(sidebarWidth-4, contentHeight-2) // -4 for border padding
	sidebarBox := statusBoxStyle.Width(sidebarWidth).Height(contentHeight).Render(sidebar)

	// Build logs panel
	logHeader := "Logs"
	if m.logViewport.TotalLineCount() > 0 {
		logHeader = fmt.Sprintf("Logs (%d lines, %d%%)", m.logViewport.TotalLineCount(), int(m.logViewport.ScrollPercent()*100))
	}

	logsContent := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(logHeader) + "\n" + m.logViewport.View()

	logsBox := logBoxStyle.Width(mainWidth).Height(contentHeight).Render(logsContent)

	// Join horizontally
	main := lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, logsBox)
	fullView := main + "\n" + m.renderHelp()

	if m.permissionDialog != nil {
		return m.permissionDialog.ViewWithBackground(m.width, m.height, fullView)
	}

	return fullView
}

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
				// Truncate last line with ellipsis
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

func (m Model) renderSidebar(width int, height int) string {
	var b strings.Builder

	b.WriteString(m.renderSidebarHeader(width))
	b.WriteString(m.renderSidebarTicket(width))
	b.WriteString(m.renderSidebarEnvironment(width))
	b.WriteString(m.renderSidebarProgress(width))
	b.WriteString(m.renderSidebarUsage(width))
	b.WriteString(m.renderSidebarPhases(width, height))
	b.WriteString(m.renderSidebarFooter())

	return b.String()
}

func (m Model) renderSidebarHeader(width int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ PROGRAMMATOR"))
	b.WriteString("\n")

	if m.guardMode {
		b.WriteString(guardStyle.Render("🛡  GUARD MODE"))
		b.WriteString("\n")
	} else if strings.Contains(m.config.ClaudeFlags, "--dangerously-skip-permissions") {
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
	case statePaused:
		return pausedStyle.Render("⏸ PAUSED")
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
	if m.workingDir == "" && m.gitBranch == "" && m.config.ClaudeConfigDir == "" {
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
	if m.config.ClaudeConfigDir != "" {
		b.WriteString(labelStyle.Render("Claude: "))
		b.WriteString(valueStyle.Render(abbreviatePath(m.config.ClaudeConfigDir)))
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

	models := make([]string, 0, len(m.state.TokensByModel))
	for model := range m.state.TokensByModel {
		models = append(models, model)
	}
	sort.Strings(models)

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

func (m Model) renderSidebarPhases(width int, height int) string {
	if m.workItem == nil || len(m.workItem.Phases) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(sectionHeader("Phases", width))
	b.WriteString("\n")
	b.WriteString(m.renderPhasesContent(width, height))

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

func (m Model) renderPhasesContent(width int, height int) string {
	var b strings.Builder

	currentPhase := m.workItem.CurrentPhase()
	currentIdx := -1
	for i, phase := range m.workItem.Phases {
		if currentPhase != nil && phase.Name == currentPhase.Name {
			currentIdx = i
			break
		}
	}

	usedLines := 8
	availableForPhases := max(5, height-usedLines)
	contextSize := max(2, (availableForPhases-2)/2)

	showFrom := 0
	showTo := len(m.workItem.Phases) - 1

	if len(m.workItem.Phases) > availableForPhases && currentIdx >= 0 {
		showFrom = max(0, currentIdx-contextSize)
		showTo = min(len(m.workItem.Phases)-1, currentIdx+contextSize)
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

	switch m.runState {
	case stateRunning:
		parts = append(parts, "p: pause", "s: stop")
	case statePaused:
		parts = append(parts, "p: resume", "s: stop")
	case stateStopped, stateComplete:
		// No controls needed for stopped/complete states
	}

	parts = append(parts, "↑/↓: scroll", "q: quit")

	return helpStyle.Render(strings.Join(parts, " • "))
}

func (m *Model) SetLoop(l *loop.Loop) {
	m.loop = l
}

func (m Model) wrapLogs() string {
	content := strings.Join(m.logs, "")
	if content == "" {
		return ""
	}

	// Process [PROG] markers before glamour
	lines := strings.Split(content, "\n")
	var processed []string
	var markdownBuffer []string

	flushMarkdown := func() {
		if len(markdownBuffer) > 0 {
			md := strings.Join(markdownBuffer, "\n")
			if m.renderer != nil {
				if rendered, err := m.renderer.Render(md); err == nil {
					processed = append(processed, strings.TrimSpace(rendered))
				} else {
					processed = append(processed, md)
				}
			} else {
				processed = append(processed, md)
			}
			markdownBuffer = nil
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "[PROG]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[PROG]")
			styled := progPrefixStyle.Render("▶ programmator:") + " " + msg
			processed = append(processed, styled)
		} else if strings.HasPrefix(line, "[TOOL]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[TOOL]")
			styled := toolStyle.Render("> " + msg)
			processed = append(processed, styled)
		} else if strings.HasPrefix(line, "[TOOLRES]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[TOOLRES]")
			processed = append(processed, toolResStyle.Render(msg))
		} else if strings.HasPrefix(line, "[DIFF+]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[DIFF+]")
			processed = append(processed, diffAddStyle.Render(msg))
		} else if strings.HasPrefix(line, "[DIFF-]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[DIFF-]")
			processed = append(processed, diffDelStyle.Render(msg))
		} else if strings.HasPrefix(line, "[DIFF@]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[DIFF@]")
			processed = append(processed, diffHunkStyle.Render(msg))
		} else if strings.HasPrefix(line, "[DIFF ]") {
			flushMarkdown()
			msg := strings.TrimPrefix(line, "[DIFF ]")
			processed = append(processed, diffCtxStyle.Render(msg))
		} else {
			markdownBuffer = append(markdownBuffer, line)
		}
	}
	flushMarkdown()

	return strings.Join(processed, "\n")
}

func getGitInfo(workingDir string) (branch string, dirty bool) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = workingDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	branch = strings.TrimSpace(out.String())

	cmd = exec.Command("git", "status", "--porcelain")
	cmd.Dir = workingDir
	out.Reset()
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return branch, false
	}
	dirty = len(strings.TrimSpace(out.String())) > 0
	return branch, dirty
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
	// claude-opus-4-5-20251101 -> opus-4-5
	// claude-sonnet-4-5-20250514 -> sonnet-4-5
	model = strings.TrimPrefix(model, "claude-")
	if idx := strings.LastIndex(model, "-20"); idx > 0 {
		model = model[:idx]
	}
	return model
}

type TUI struct {
	program                *tea.Program
	model                  Model
	interactivePermissions bool
	guardMode              bool
	allowPatterns          []string
	skipReview             bool
	reviewOnly             bool
	reviewConfig           *review.Config
	progressLogger         *progress.Logger
	gitWorkflowConfig      *loop.GitWorkflowConfig
	ticketCommand          string
}

func New(config safety.Config) *TUI {
	timing.Log("TUI.New: start")
	model := NewModel(config)
	timing.Log("TUI.New: model created")
	return &TUI{
		model:                  model,
		interactivePermissions: true,
	}
}

func (t *TUI) SetInteractivePermissions(enabled bool) {
	t.interactivePermissions = enabled
}

func (t *TUI) SetGuardMode(enabled bool) {
	t.guardMode = enabled
	t.model.guardMode = enabled
}

func (t *TUI) SetAllowPatterns(patterns []string) {
	t.allowPatterns = patterns
}

func (t *TUI) SetSkipReview(skip bool) {
	t.skipReview = skip
}

func (t *TUI) SetReviewOnly(reviewOnly bool) {
	t.reviewOnly = reviewOnly
}

func (t *TUI) SetReviewConfig(cfg review.Config) {
	t.reviewConfig = &cfg
}

// SetProgressLogger sets the progress logger for persistent log files.
func (t *TUI) SetProgressLogger(logger *progress.Logger) {
	t.progressLogger = logger
}

// SetGitWorkflowConfig sets the git workflow configuration.
func (t *TUI) SetGitWorkflowConfig(cfg loop.GitWorkflowConfig) {
	t.gitWorkflowConfig = &cfg
}

func (t *TUI) SetTicketCommand(cmd string) {
	t.ticketCommand = cmd
}

func (t *TUI) Run(ticketID string, workingDir string) (*loop.Result, error) {
	timing.Log("TUI.Run: start")

	t.model.workingDir = workingDir
	t.model.gitBranch, t.model.gitDirty = getGitInfo(workingDir)

	outputChan := make(chan string, 100)
	stateChan := make(chan TicketUpdateMsg, 10)
	doneChan := make(chan LoopDoneMsg, 1)
	processStatsChan := make(chan ProcessStatsMsg, 10)
	permissionChan := make(chan PermissionRequestMsg, 1)

	timing.Log("TUI.Run: channels created")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var permServer *permission.Server
	if t.interactivePermissions {
		var err error
		permServer, err = permission.NewServer(workingDir, func(req *permission.Request) permission.HandlerResponse {
			respChan := make(chan permission.HandlerResponse, 1)
			permissionChan <- PermissionRequestMsg{Request: req, ResponseChan: respChan}
			return <-respChan
		})
		if err != nil {
			return nil, fmt.Errorf("failed to start permission server: %w", err)
		}
		defer permServer.Close()

		// Build pre-allowed patterns
		preAllowed := append([]string{}, t.allowPatterns...)

		// Auto-allow read-only access to the current repo
		if repoRoot := getGitRoot(workingDir); repoRoot != "" {
			preAllowed = append(preAllowed,
				fmt.Sprintf("Read(%s/**)", repoRoot),
				// Glob/Grep tool input is the search pattern, not a file path,
				// so path-based patterns don't match. Allow all Glob/Grep since
				// they are read-only operations.
				"Glob",
				"Grep",
			)
		}

		if len(preAllowed) > 0 {
			permServer.SetPreAllowed(preAllowed)
		}

		go func() { _ = permServer.Serve(ctx) }()
		timing.Log("TUI.Run: permission server started")
	}

	l := loop.New(
		t.model.config,
		workingDir,
		func(text string) {
			select {
			case outputChan <- text:
			default:
			}
		},
		func(state *safety.State, workItem *source.WorkItem, filesChanged []string) {
			select {
			case stateChan <- TicketUpdateMsg{
				WorkItem:     workItem,
				State:        state,
				FilesChanged: filesChanged,
			}:
			default:
			}
		},
		true,
	)
	l.SetProcessStatsCallback(func(pid int, memoryKB int64) {
		select {
		case processStatsChan <- ProcessStatsMsg{PID: pid, MemoryKB: memoryKB}:
		default:
		}
	})
	if permServer != nil {
		l.SetPermissionSocketPath(permServer.SocketPath())
	}
	l.SetGuardMode(t.guardMode)
	l.SetSkipReview(t.skipReview)
	l.SetReviewOnly(t.reviewOnly)
	if t.reviewConfig != nil {
		l.SetReviewConfig(*t.reviewConfig)
	}
	if t.progressLogger != nil {
		l.SetProgressLogger(t.progressLogger)
	}
	if t.gitWorkflowConfig != nil {
		l.SetGitWorkflowConfig(*t.gitWorkflowConfig)
	}
	if t.ticketCommand != "" {
		l.SetTicketCommand(t.ticketCommand)
	}

	t.model.SetLoop(l)
	timing.Log("TUI.Run: loop created")

	t.program = tea.NewProgram(t.model, tea.WithAltScreen())
	timing.Log("TUI.Run: tea.Program created")

	go func() {
		timing.Log("TUI.Run: loop goroutine started")
		result, err := l.Run(ticketID)
		doneChan <- LoopDoneMsg{Result: result, Err: err}
	}()

	go func() {
		for {
			select {
			case text := <-outputChan:
				t.program.Send(LogMsg{Text: text})
			case update := <-stateChan:
				t.program.Send(update)
			case stats := <-processStatsChan:
				t.program.Send(stats)
			case perm := <-permissionChan:
				t.program.Send(perm)
			case done := <-doneChan:
				t.program.Send(done)
				return
			}
		}
	}()

	timing.Log("TUI.Run: starting tea.Program.Run")
	finalModel, err := t.program.Run()
	timing.Log("TUI.Run: tea.Program.Run returned")
	if err != nil {
		return nil, err
	}

	m := finalModel.(Model)
	if m.err != nil {
		return m.result, m.err
	}
	return m.result, nil
}

// getGitRoot returns the git repository root for the given directory, or empty string if not in a repo.
func getGitRoot(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
