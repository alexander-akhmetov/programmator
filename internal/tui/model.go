package tui

import (
	"math/rand/v2"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

type runState int

const (
	stateRunning runState = iota
	stateStopped
	stateComplete
)

// Model is the bubbletea model for the TUI.
type Model struct {
	workItem        *domain.WorkItem
	state           *safety.State
	config          safety.Config
	filesChanged    []string
	logs            []string
	events          []event.Event
	logViewport     viewport.Model
	spinner         spinner.Model
	width           int
	height          int
	runState        runState
	loop            *loop.Loop
	ready           bool
	result          *loop.Result
	err             error
	renderer        *glamour.TermRenderer
	workingDir      string
	gitBranch       string
	gitDirty        bool
	claudePID       int
	claudeMemKB     int64
	tipIndex        int
	hideTips        bool
	claudeFlags     string
	claudeConfigDir string
}

// NewModel creates a new Model with the given safety config.
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
		tipIndex: rand.IntN(len(sidebarTips)),
	}
}

// SetLoop sets the loop instance that the model can control (pause/stop).
func (m *Model) SetLoop(l *loop.Loop) {
	m.loop = l
}

// TicketUpdateMsg carries work-item and state updates from the loop.
type TicketUpdateMsg struct {
	WorkItem     *domain.WorkItem
	State        *safety.State
	FilesChanged []string
}

// LogMsg carries a raw log line (legacy path).
type LogMsg struct {
	Text string
}

// LoopDoneMsg signals the loop has finished.
type LoopDoneMsg struct {
	Result *loop.Result
	Err    error
}

// ProcessStatsMsg carries Claude process stats (PID, memory).
type ProcessStatsMsg struct {
	PID      int
	MemoryKB int64
}

// EventMsg carries a typed event from the loop.
type EventMsg struct {
	Event event.Event
}

type rendererReadyMsg struct {
	renderer *glamour.TermRenderer
}
