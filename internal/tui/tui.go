// Package tui implements the terminal user interface using bubbletea.
package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	gitutil "github.com/alexander-akhmetov/programmator/internal/git"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/permission"
	"github.com/alexander-akhmetov/programmator/internal/progress"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/timing"
)

// TUI wraps the bubbletea program and the orchestration loop.
type TUI struct {
	program                *tea.Program
	model                  Model
	interactivePermissions bool
	guardMode              bool
	allowPatterns          []string
	reviewOnly             bool
	reviewConfig           *review.Config
	progressLogger         *progress.Logger
	promptBuilder          *prompt.Builder
	gitWorkflowConfig      *loop.GitWorkflowConfig
	codexConfig            *config.CodexConfig
	ticketCommand          string
}

// New creates a new TUI with the given safety config.
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

func (t *TUI) SetReviewOnly(reviewOnly bool) {
	t.reviewOnly = reviewOnly
}

func (t *TUI) SetReviewConfig(cfg review.Config) {
	t.reviewConfig = &cfg
}

func (t *TUI) SetProgressLogger(logger *progress.Logger) {
	t.progressLogger = logger
}

func (t *TUI) SetGitWorkflowConfig(cfg loop.GitWorkflowConfig) {
	t.gitWorkflowConfig = &cfg
}

func (t *TUI) SetPromptBuilder(b *prompt.Builder) {
	t.promptBuilder = b
}

func (t *TUI) SetCodexConfig(cfg config.CodexConfig) {
	t.codexConfig = &cfg
}

func (t *TUI) SetTicketCommand(cmd string) {
	t.ticketCommand = cmd
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

		preAllowed := append([]string{}, t.allowPatterns...)

		if repoRoot := getGitRoot(workingDir); repoRoot != "" {
			preAllowed = append(preAllowed,
				fmt.Sprintf("Read(%s/**)", repoRoot),
				"Glob",
				"Grep",
			)
		}

		if len(preAllowed) > 0 {
			permServer.SetPreAllowed(preAllowed)
		}

		go func() {
			if err := permServer.Serve(ctx); err != nil {
				debug.Logf("permission server error: %v", err)
			}
		}()
		timing.Log("TUI.Run: permission server started")
	}

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
	if permServer != nil {
		l.SetPermissionSocketPath(permServer.SocketPath())
	}
	l.SetGuardMode(t.guardMode)
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
	if t.promptBuilder != nil {
		l.SetPromptBuilder(t.promptBuilder)
	}
	if t.codexConfig != nil {
		l.SetCodexConfig(*t.codexConfig)
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
			case ev := <-eventChan:
				t.program.Send(EventMsg{Event: ev})
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
	repo, err := gitutil.NewRepo(dir)
	if err != nil {
		return ""
	}
	return repo.WorkDir()
}
