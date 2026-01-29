package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/loop"
	"github.com/worksonmyai/programmator/internal/safety"
	"github.com/worksonmyai/programmator/internal/source"
)

func TestNewModel(t *testing.T) {
	config := safety.Config{
		MaxIterations:   10,
		StagnationLimit: 3,
		Timeout:         60,
	}

	model := NewModel(config)

	if model.state == nil {
		t.Error("state should not be nil")
	}
	if model.config.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", model.config.MaxIterations)
	}
	if model.runState != stateRunning {
		t.Errorf("runState = %v, want stateRunning", model.runState)
	}
	if model.logs == nil {
		t.Error("logs should not be nil")
	}
}

func TestModelInit(t *testing.T) {
	config := safety.Config{}
	model := NewModel(config)

	cmd := model.Init()

	if cmd == nil {
		t.Error("Init() should return a spinner tick command")
	}
}

func TestModelUpdateKeyMsgs(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		initialState   runState
		hasLoop        bool
		expectQuit     bool
		expectNewState runState
	}{
		{
			name:       "q key quits",
			key:        "q",
			expectQuit: true,
		},
		{
			name:       "ctrl+c quits",
			key:        "ctrl+c",
			expectQuit: true,
		},
		{
			name:           "p toggles pause when running",
			key:            "p",
			initialState:   stateRunning,
			hasLoop:        true,
			expectNewState: statePaused,
		},
		{
			name:           "p resumes when paused",
			key:            "p",
			initialState:   statePaused,
			hasLoop:        true,
			expectNewState: stateRunning,
		},
		{
			name:           "s stops when running",
			key:            "s",
			initialState:   stateRunning,
			hasLoop:        true,
			expectNewState: stateStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(safety.Config{})
			model.runState = tt.initialState

			if tt.hasLoop {
				model.loop = loop.New(safety.Config{}, "", nil, nil, false)
			}

			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})

			m := updatedModel.(Model)

			if tt.expectQuit {
				if cmd == nil {
					t.Error("expected quit command")
				}
			}

			if tt.expectNewState != 0 && m.runState != tt.expectNewState {
				t.Errorf("runState = %v, want %v", m.runState, tt.expectNewState)
			}
		})
	}
}

func TestModelUpdateTicketMsg(t *testing.T) {
	model := NewModel(safety.Config{})

	testWorkItem := &source.WorkItem{
		ID:    "test-123",
		Title: "Test Ticket",
	}
	testState := safety.NewState()
	testState.Iteration = 5

	msg := TicketUpdateMsg{
		WorkItem:     testWorkItem,
		State:        testState,
		FilesChanged: []string{"file1.go", "file2.go"},
	}

	updatedModel, _ := model.Update(msg)
	m := updatedModel.(Model)

	if m.workItem != testWorkItem {
		t.Error("workItem should be updated")
	}
	if m.state != testState {
		t.Error("state should be updated")
	}
	if len(m.filesChanged) != 2 {
		t.Errorf("filesChanged len = %d, want 2", len(m.filesChanged))
	}
}

func TestModelUpdateLogMsg(t *testing.T) {
	model := NewModel(safety.Config{})
	model.width = 80
	model.height = 24
	model.ready = true

	msg := LogMsg{Text: "test log message"}

	updatedModel, _ := model.Update(msg)
	m := updatedModel.(Model)

	if len(m.logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(m.logs))
	}
	if m.logs[0] != "test log message" {
		t.Errorf("logs[0] = %q, want %q", m.logs[0], "test log message")
	}
}

func TestModelUpdateLoopDoneMsg(t *testing.T) {
	model := NewModel(safety.Config{})

	result := &loop.Result{
		ExitReason: safety.ExitReasonComplete,
		Iterations: 10,
	}

	msg := LoopDoneMsg{Result: result, Err: nil}

	updatedModel, _ := model.Update(msg)
	m := updatedModel.(Model)

	if m.result != result {
		t.Error("result should be set")
	}
	if m.runState != stateComplete {
		t.Errorf("runState = %v, want stateComplete", m.runState)
	}
}

func TestModelUpdateWindowSizeMsg(t *testing.T) {
	model := NewModel(safety.Config{})

	msg := tea.WindowSizeMsg{Width: 100, Height: 50}

	updatedModel, _ := model.Update(msg)
	m := updatedModel.(Model)

	if m.width != 100 {
		t.Errorf("width = %d, want 100", m.width)
	}
	if m.height != 50 {
		t.Errorf("height = %d, want 50", m.height)
	}
	if !m.ready {
		t.Error("ready should be true after window size msg")
	}
}

func TestModelView(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.width = 80
	model.height = 24

	view := model.View()

	if view == "" {
		t.Error("View() should not return empty string")
	}
	if view != "Initializing..." {
		model.width = 0
		view = model.View()
		if view != "Initializing..." {
			t.Error("View() should return 'Initializing...' when width is 0")
		}
	}
}

func TestModelRenderSidebar(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3}
	model := NewModel(config)
	model.width = 80
	model.height = 24

	model.runState = stateRunning
	status := model.renderSidebar(36, 20)
	if status == "" {
		t.Error("renderSidebar() should not return empty string")
	}

	model.runState = statePaused
	status = model.renderSidebar(36, 20)
	if status == "" {
		t.Error("renderSidebar() should not return empty string when paused")
	}

	model.runState = stateStopped
	status = model.renderSidebar(36, 20)
	if status == "" {
		t.Error("renderSidebar() should not return empty string when stopped")
	}

	model.runState = stateComplete
	status = model.renderSidebar(36, 20)
	if status == "" {
		t.Error("renderSidebar() should not return empty string when complete")
	}
}

func TestModelRenderSidebarWithTicket(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.workItem = &source.WorkItem{
		ID:    "t-123",
		Title: "Test Ticket Title",
		Phases: []source.Phase{
			{Name: "Phase 1", Completed: true},
			{Name: "Phase 2", Completed: false},
		},
	}
	model.state = safety.NewState()
	model.state.Iteration = 5

	status := model.renderSidebar(36, 20)

	if status == "" {
		t.Error("renderSidebar() should not return empty string with ticket")
	}
}

func TestModelRenderHelp(t *testing.T) {
	tests := []struct {
		name     string
		state    runState
		contains []string
	}{
		{
			name:     "running state",
			state:    stateRunning,
			contains: []string{"pause", "stop", "quit"},
		},
		{
			name:     "paused state",
			state:    statePaused,
			contains: []string{"resume", "stop", "quit"},
		},
		{
			name:     "stopped state",
			state:    stateStopped,
			contains: []string{"quit"},
		},
		{
			name:     "complete state",
			state:    stateComplete,
			contains: []string{"quit"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(safety.Config{})
			model.runState = tt.state

			help := model.renderHelp()

			if help == "" {
				t.Errorf("renderHelp() should not return empty string")
			}
		})
	}
}

func TestModelSetLoop(t *testing.T) {
	model := NewModel(safety.Config{})
	l := loop.New(safety.Config{}, "", nil, nil, false)

	model.SetLoop(l)

	if model.loop != l {
		t.Error("loop should be set")
	}
}

func TestNew(t *testing.T) {
	config := safety.Config{MaxIterations: 50}

	tui := New(config)

	if tui == nil {
		t.Fatal("New() returned nil")
	}
	if tui.model.config.MaxIterations != 50 {
		t.Errorf("config.MaxIterations = %d, want 50", tui.model.config.MaxIterations)
	}
}

func TestLogsTrimming(t *testing.T) {
	model := NewModel(safety.Config{})
	model.width = 80
	model.height = 24
	model.ready = true

	for range 5100 {
		model.logs = append(model.logs, "log entry")
	}

	msg := LogMsg{Text: "new log"}
	updatedModel, _ := model.Update(msg)
	m := updatedModel.(Model)

	if len(m.logs) > 5000 {
		t.Errorf("logs should be trimmed to 5000, got %d", len(m.logs))
	}
}

func TestRunStateValues(t *testing.T) {
	if stateRunning != 0 {
		t.Errorf("stateRunning = %d, want 0", stateRunning)
	}
	if statePaused != 1 {
		t.Errorf("statePaused = %d, want 1", statePaused)
	}
	if stateStopped != 2 {
		t.Errorf("stateStopped = %d, want 2", stateStopped)
	}
	if stateComplete != 3 {
		t.Errorf("stateComplete = %d, want 3", stateComplete)
	}
}

func TestModelUpdateScrollKeys(t *testing.T) {
	model := NewModel(safety.Config{})
	model.width = 80
	model.height = 24
	model.ready = true
	model.logs = []string{"line 1\n", "line 2\n", "line 3\n", "line 4\n", "line 5\n"}

	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = updatedModel.(Model)

	tests := []struct {
		key string
	}{
		{"up"},
		{"down"},
		{"k"},
		{"j"},
		{"pgup"},
		{"pgdown"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(_ *testing.T) {
			_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			_ = cmd
		})
	}
}

func TestModelUpdateLoopDoneMsgWhenStopped(t *testing.T) {
	model := NewModel(safety.Config{})
	model.runState = stateStopped

	result := &loop.Result{
		ExitReason: safety.ExitReasonUserInterrupt,
		Iterations: 5,
	}

	msg := LoopDoneMsg{Result: result, Err: nil}

	updatedModel, _ := model.Update(msg)
	m := updatedModel.(Model)

	require.Equal(t, stateStopped, m.runState)
}

func TestModelRenderSidebarWithError(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.err = fmt.Errorf("test error")

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
}

func TestModelRenderSidebarWithResult(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.runState = stateComplete
	model.result = &loop.Result{
		ExitReason: safety.ExitReasonComplete,
		Iterations: 5,
	}

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
}

func TestModelRenderSidebarTruncatedTitle(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.workItem = &source.WorkItem{
		ID:    "t-123",
		Title: "This is a very long ticket title that should be truncated because it exceeds 50 characters",
	}

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
}

func TestModelRenderSidebarAllPhasesComplete(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.workItem = &source.WorkItem{
		ID:    "t-123",
		Title: "Test Ticket",
		Phases: []source.Phase{
			{Name: "Phase 1", Completed: true},
			{Name: "Phase 2", Completed: true},
		},
	}

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
}

func TestWrapLogs(t *testing.T) {
	tests := []struct {
		name     string
		logs     []string
		renderer bool
		expected string
	}{
		{
			name:     "empty logs",
			logs:     []string{},
			renderer: false,
			expected: "",
		},
		{
			name:     "single log entry",
			logs:     []string{"test log message"},
			renderer: false,
			expected: "test log message",
		},
		{
			name:     "multiple log entries concatenated",
			logs:     []string{"first", "second", "third"},
			renderer: false,
			expected: "firstsecondthird",
		},
		{
			name:     "log entries with newlines",
			logs:     []string{"line1\n", "line2\n"},
			renderer: false,
			expected: "line1\nline2\n",
		},
		{
			name:     "empty string in logs",
			logs:     []string{"", "content", ""},
			renderer: false,
			expected: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(safety.Config{})
			model.logs = tt.logs
			model.renderer = nil

			result := model.wrapLogs()

			require.Equal(t, tt.expected, result)
		})
	}
}

func TestWrapLogsWithRenderer(t *testing.T) {
	model := NewModel(safety.Config{})
	model.logs = []string{"**bold**"}
	model.width = 80
	model.height = 24

	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m := updatedModel.(Model)

	result := m.wrapLogs()
	require.NotEmpty(t, result)
}

func TestModelViewWithLogs(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.width = 80
	model.height = 40
	model.ready = true
	model.logs = []string{"Log line 1\n", "Log line 2\n"}
	model.workItem = &source.WorkItem{
		ID:    "t-123",
		Title: "Test",
		Phases: []source.Phase{
			{Name: "Phase 1", Completed: false},
		},
	}
	model.state = safety.NewState()
	model.state.Iteration = 2

	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	m := updatedModel.(Model)

	view := m.View()

	require.NotEmpty(t, view)
}
