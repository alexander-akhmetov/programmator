package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/worksonmyai/programmator/internal/debug"
	"github.com/worksonmyai/programmator/internal/event"
	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/timing"
)

func createRendererCmd(width int) tea.Cmd {
	return func() tea.Msg {
		viewportWidth := max(width-6, 40)
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(viewportWidth),
		)
		if err != nil {
			debug.Logf("tui: failed to create glamour renderer: %v", err)
		}
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

		sidebarWidth := max(45, min(60, m.width*40/100))
		mainWidth := m.width - sidebarWidth - 4
		contentHeight := m.height - 3

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

	case EventMsg:
		m.events = append(m.events, msg.Event)
		if len(m.events) > 12000 {
			m.events = m.events[len(m.events)-10000:]
		}
		atBottom := m.logViewport.AtBottom()
		m.logViewport.SetContent(m.wrapLogs())
		if atBottom {
			m.logViewport.GotoBottom()
		}

	case LogMsg:
		m.logs = append(m.logs, msg.Text)
		if len(m.logs) > 5000 {
			m.logs = m.logs[len(m.logs)-5000:]
		}
		atBottom := m.logViewport.AtBottom()
		m.logViewport.SetContent(m.wrapLogs())
		if atBottom {
			m.logViewport.GotoBottom()
		}

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
			exitText := fmt.Sprintf("Loop finished: %s", msg.Result.ExitReason)
			if msg.Result.ExitMessage != "" {
				exitText += fmt.Sprintf(" (%s)", msg.Result.ExitMessage)
			}
			if len(m.events) > 0 {
				m.events = append(m.events, event.Prog(exitText))
			} else {
				m.logs = append(m.logs, fmt.Sprintf("\n"+protocol.MarkerProg+"%s\n", exitText))
			}
			atBottom := m.logViewport.AtBottom()
			m.logViewport.SetContent(m.wrapLogs())
			if atBottom {
				m.logViewport.GotoBottom()
			}
		}
		if msg.Err != nil {
			errText := fmt.Sprintf("Loop error: %v", msg.Err)
			if len(m.events) > 0 {
				m.events = append(m.events, event.Prog(errText))
			} else {
				m.logs = append(m.logs, fmt.Sprintf("\n"+protocol.MarkerProg+"%s\n", msg.Err))
			}
			atBottom := m.logViewport.AtBottom()
			m.logViewport.SetContent(m.wrapLogs())
			if atBottom {
				m.logViewport.GotoBottom()
			}
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}
