// Package tui implements the terminal user interface using bubbletea.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/timing"
)

// TUI wraps the bubbletea program and the orchestration loop.
type TUI struct {
	program           *tea.Program
	model             Model
	reviewOnly        bool
	reviewConfig      *review.Config
	promptBuilder     *prompt.Builder
	gitWorkflowConfig *loop.GitWorkflowConfig
	ticketCommand     string
	executorConfig    *llm.ExecutorConfig
}

// New creates a new TUI with the given safety config.
func New(config safety.Config) *TUI {
	timing.Log("TUI.New: start")
	model := NewModel(config)
	timing.Log("TUI.New: model created")
	return &TUI{
		model: model,
	}
}

func (t *TUI) SetReviewOnly(reviewOnly bool) {
	t.reviewOnly = reviewOnly
}

func (t *TUI) SetReviewConfig(cfg review.Config) {
	t.reviewConfig = &cfg
}

func (t *TUI) SetGitWorkflowConfig(cfg loop.GitWorkflowConfig) {
	t.gitWorkflowConfig = &cfg
}

func (t *TUI) SetPromptBuilder(b *prompt.Builder) {
	t.promptBuilder = b
}

func (t *TUI) SetTicketCommand(cmd string) {
	t.ticketCommand = cmd
}

func (t *TUI) SetHideTips(hide bool) {
	t.model.hideTips = hide
}

func (t *TUI) SetExecutorConfig(cfg llm.ExecutorConfig) {
	t.executorConfig = &cfg
	t.model.claudeConfigDir = cfg.Claude.ClaudeConfigDir
}

// Run starts the TUI and the orchestration loop, blocking until done.
func (t *TUI) Run(ticketID string, workingDir string) (*loop.Result, error) {
	timing.Log("TUI.Run: start")

	t.model.workingDir = workingDir
	t.model.gitBranch, t.model.gitDirty = getGitInfo(workingDir)

	eventChan := make(chan event.Event, 200)
	stateChan := make(chan TicketUpdateMsg, 10)
	doneChan := make(chan LoopDoneMsg, 1)
	processStatsChan := make(chan ProcessStatsMsg, 10)

	timing.Log("TUI.Run: channels created")

	l := loop.New(
		t.model.config,
		workingDir,
		nil,
		func(state *safety.State, workItem *domain.WorkItem, filesChanged []string) {
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
	l.SetEventCallback(func(ev event.Event) {
		select {
		case eventChan <- ev:
		default:
		}
	})
	l.SetProcessStatsCallback(func(pid int, memoryKB int64) {
		select {
		case processStatsChan <- ProcessStatsMsg{PID: pid, MemoryKB: memoryKB}:
		default:
		}
	})
	l.SetReviewOnly(t.reviewOnly)
	if t.reviewConfig != nil {
		l.SetReviewConfig(*t.reviewConfig)
	}
	if t.gitWorkflowConfig != nil {
		l.SetGitWorkflowConfig(*t.gitWorkflowConfig)
	}
	if t.promptBuilder != nil {
		l.SetPromptBuilder(t.promptBuilder)
	}
	if t.ticketCommand != "" {
		l.SetTicketCommand(t.ticketCommand)
	}
	if t.executorConfig != nil {
		l.SetExecutorConfig(*t.executorConfig)
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
			case ev := <-eventChan:
				t.program.Send(EventMsg{Event: ev})
			case update := <-stateChan:
				t.program.Send(update)
			case stats := <-processStatsChan:
				t.program.Send(stats)
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
