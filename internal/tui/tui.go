// Package tui implements the terminal user interface using bubbletea.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/ticket"
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

	pausedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Bold(true)

	runningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	stoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
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
	ticket       *ticket.Ticket
	state        *safety.State
	config       safety.Config
	filesChanged []string
	logs         []string
	logViewport  viewport.Model
	spinner      spinner.Model
	width        int
	height       int
	runState     runState
	loop         *loop.Loop
	ready        bool
	result       *loop.Result
	err          error
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
	Ticket       *ticket.Ticket
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

func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
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

		case "up", "k":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			cmds = append(cmds, cmd)

		case "down", "j":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			cmds = append(cmds, cmd)

		case "pgup":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			cmds = append(cmds, cmd)

		case "pgdown":
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		logHeight := max(m.height-18, 5)

		if !m.ready {
			m.logViewport = viewport.New(msg.Width-6, logHeight)
			m.logViewport.SetContent(m.wrapLogs())
			m.ready = true
		} else {
			m.logViewport.Width = msg.Width - 6
			m.logViewport.Height = logHeight
			m.logViewport.SetContent(m.wrapLogs())
		}

	case TicketUpdateMsg:
		m.ticket = msg.Ticket
		m.state = msg.State
		m.filesChanged = msg.FilesChanged

	case LogMsg:
		m.logs = append(m.logs, msg.Text)
		if len(m.logs) > 5000 {
			m.logs = m.logs[len(m.logs)-5000:]
		}
		m.logViewport.SetContent(m.wrapLogs())
		m.logViewport.GotoBottom()

	case LoopDoneMsg:
		m.result = msg.Result
		m.err = msg.Err
		if m.runState != stateStopped {
			m.runState = stateComplete
		}

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

	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ PROGRAMMATOR"))
	b.WriteString("\n")

	statusContent := m.renderStatus()
	statusWidth := max(m.width-4, 40)
	b.WriteString(statusBoxStyle.Width(statusWidth).Render(statusContent))
	b.WriteString("\n\n")

	logHeight := max(m.height-18, 5)

	logHeader := "─ Logs "
	if m.logViewport.TotalLineCount() > 0 {
		logHeader += fmt.Sprintf("(%d lines, %d%% scrolled) ", m.logViewport.TotalLineCount(), int(m.logViewport.ScrollPercent()*100))
	}

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(logHeader))
	b.WriteString("\n")
	b.WriteString(logBoxStyle.Width(statusWidth).Height(logHeight).Render(m.logViewport.View()))
	b.WriteString("\n\n")

	b.WriteString(m.renderHelp())

	return b.String()
}

func (m Model) renderStatus() string {
	var b strings.Builder

	var stateIndicator string
	switch m.runState {
	case stateRunning:
		stateIndicator = runningStyle.Render(m.spinner.View() + " Running")
	case statePaused:
		stateIndicator = pausedStyle.Render("⏸ PAUSED")
	case stateStopped:
		stateIndicator = stoppedStyle.Render("⏹ STOPPED")
	case stateComplete:
		stateIndicator = runningStyle.Render("✓ COMPLETE")
	}

	b.WriteString(stateIndicator)
	b.WriteString("\n\n")

	if m.ticket != nil {
		b.WriteString(labelStyle.Render("Ticket: "))
		b.WriteString(valueStyle.Render(m.ticket.ID))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Title:  "))
		title := m.ticket.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		b.WriteString(valueStyle.Render(title))
		b.WriteString("\n")
	} else {
		b.WriteString(labelStyle.Render("Ticket: "))
		b.WriteString(valueStyle.Render("-"))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if m.state != nil {
		b.WriteString(labelStyle.Render("Iteration:  "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("%d/%d", m.state.Iteration, m.config.MaxIterations)))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Stagnation: "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("%d/%d", m.state.ConsecutiveNoChanges, m.config.StagnationLimit)))
		b.WriteString("\n")
		b.WriteString(labelStyle.Render("Files:      "))
		b.WriteString(valueStyle.Render(fmt.Sprintf("%d changed", len(m.state.TotalFilesChanged))))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	if m.ticket != nil {
		phase := m.ticket.CurrentPhase()
		b.WriteString(labelStyle.Render("Phase: "))
		if phase != nil {
			b.WriteString(phaseStyle.Render(phase.Name))
		} else {
			b.WriteString(runningStyle.Render("All complete!"))
		}
	}

	if m.result != nil && m.runState == stateComplete {
		b.WriteString("\n\n")
		b.WriteString(labelStyle.Render("Exit: "))
		b.WriteString(valueStyle.Render(string(m.result.ExitReason)))
	}

	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(stoppedStyle.Render(fmt.Sprintf("Error: %v", m.err)))
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
	if m.logViewport.Width <= 0 {
		return strings.Join(m.logs, "")
	}

	var wrapped strings.Builder
	wrapWidth := m.logViewport.Width

	for _, log := range m.logs {
		lines := strings.Split(log, "\n")
		for i, line := range lines {
			if len(line) <= wrapWidth {
				wrapped.WriteString(line)
			} else {
				for len(line) > wrapWidth {
					wrapped.WriteString(line[:wrapWidth])
					wrapped.WriteString("\n")
					line = line[wrapWidth:]
				}
				wrapped.WriteString(line)
			}
			if i < len(lines)-1 {
				wrapped.WriteString("\n")
			}
		}
	}
	return wrapped.String()
}

type TUI struct {
	program *tea.Program
	model   Model
}

func New(config safety.Config) *TUI {
	model := NewModel(config)
	return &TUI{
		model: model,
	}
}

func (t *TUI) Run(ticketID string, workingDir string) (*loop.Result, error) {
	outputChan := make(chan string, 100)
	stateChan := make(chan TicketUpdateMsg, 10)
	doneChan := make(chan LoopDoneMsg, 1)

	l := loop.New(
		t.model.config,
		workingDir,
		func(text string) {
			select {
			case outputChan <- text:
			default:
			}
		},
		func(state *safety.State, tkt *ticket.Ticket, filesChanged []string) {
			select {
			case stateChan <- TicketUpdateMsg{
				Ticket:       tkt,
				State:        state,
				FilesChanged: filesChanged,
			}:
			default:
			}
		},
		true,
	)

	t.model.SetLoop(l)

	t.program = tea.NewProgram(t.model, tea.WithAltScreen())

	go func() {
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
			case done := <-doneChan:
				t.program.Send(done)
				return
			}
		}
	}()

	finalModel, err := t.program.Run()
	if err != nil {
		return nil, err
	}

	m := finalModel.(Model)
	if m.err != nil {
		return m.result, m.err
	}
	return m.result, nil
}
