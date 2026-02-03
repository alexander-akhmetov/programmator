package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

var tipSnippets = []string{
	"plan create",
	"plan.md",
	"--auto-commit",
	"logs --follow",
	"config show",
	"Press `p`",
	".programmator/prompts/",
	"Guard mode",
}

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

	testWorkItem := &domain.WorkItem{
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

func TestLogMsgPreservesScrollPosition(t *testing.T) {
	model := NewModel(safety.Config{})
	// Initialize viewport via WindowSizeMsg
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m := updated.(Model)

	// Add enough logs to make viewport scrollable
	for i := range 100 {
		updated, _ = m.Update(LogMsg{Text: fmt.Sprintf("line %d\n", i)})
		m = updated.(Model)
	}
	require.True(t, m.logViewport.AtBottom(), "should be at bottom after sequential appends")

	// Scroll up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	require.False(t, m.logViewport.AtBottom(), "should not be at bottom after scrolling up")

	yBefore := m.logViewport.YOffset

	// New log arrives — scroll position must be preserved
	updated, _ = m.Update(LogMsg{Text: "new message\n"})
	m = updated.(Model)

	require.Equal(t, yBefore, m.logViewport.YOffset, "scroll position should be preserved when user scrolled up")
	require.False(t, m.logViewport.AtBottom(), "should still not be at bottom")
}

func TestLoopDoneMsgPreservesScrollPosition(t *testing.T) {
	model := NewModel(safety.Config{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m := updated.(Model)

	for i := range 100 {
		updated, _ = m.Update(LogMsg{Text: fmt.Sprintf("line %d\n", i)})
		m = updated.(Model)
	}

	// Scroll up
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)

	yBefore := m.logViewport.YOffset

	// Loop finishes — scroll position must be preserved
	result := &loop.Result{ExitReason: safety.ExitReasonComplete, Iterations: 5}
	updated, _ = m.Update(LoopDoneMsg{Result: result})
	m = updated.(Model)

	require.Equal(t, yBefore, m.logViewport.YOffset, "scroll position should be preserved when user scrolled up")
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
	tests := []struct {
		name     string
		runState runState
		contains []string
	}{
		{
			name:     "running state",
			runState: stateRunning,
			contains: []string{"Tips", "Did you know?"},
		},
		{
			name:     "paused state",
			runState: statePaused,
			contains: []string{"Tips"},
		},
		{
			name:     "stopped state",
			runState: stateStopped,
			contains: []string{"Tips"},
		},
		{
			name:     "complete state",
			runState: stateComplete,
			contains: []string{"Tips"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := safety.Config{MaxIterations: 10, StagnationLimit: 3}
			model := NewModel(config)
			model.width = 80
			model.height = 24
			model.runState = tt.runState

			status := model.renderSidebar(36, 20)
			require.NotEmpty(t, status)
			for _, s := range tt.contains {
				require.Contains(t, status, s)
			}
		})
	}
}

func TestModelRenderSidebarWithTicket(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.workItem = &domain.WorkItem{
		ID:    "t-123",
		Title: "Test Ticket Title",
		Phases: []domain.Phase{
			{Name: "Phase 1", Completed: true},
			{Name: "Phase 2", Completed: false},
		},
	}
	model.state = safety.NewState()
	model.state.Iteration = 5

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
	require.Contains(t, status, "Tips")
	require.Contains(t, status, "Did you know?")
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
		t.Run(tt.key, func(t *testing.T) {
			updatedModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			_ = cmd
			m := updatedModel.(Model)
			if m.width != 80 || m.height != 24 {
				t.Errorf("model dimensions changed: got %dx%d", m.width, m.height)
			}
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
	model.workItem = &domain.WorkItem{
		ID:    "t-123",
		Title: "This is a very long ticket title that should be truncated because it exceeds 50 characters",
	}

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
}

func TestModelRenderSidebarAllPhasesComplete(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 10, StagnationLimit: 3})
	model.workItem = &domain.WorkItem{
		ID:    "t-123",
		Title: "Test Ticket",
		Phases: []domain.Phase{
			{Name: "Phase 1", Completed: true},
			{Name: "Phase 2", Completed: true},
		},
	}

	status := model.renderSidebar(36, 20)

	require.NotEmpty(t, status)
}

func TestModelRenderSidebarManyPhasesOverflow(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 50, StagnationLimit: 3})

	// Create 30 phases with current phase in the middle
	phases := make([]domain.Phase, 30)
	for i := range phases {
		phases[i] = domain.Phase{
			Name:      fmt.Sprintf("Phase %d: Task description", i+1),
			Completed: i < 15, // First 15 completed, current is 16
		}
	}

	model.workItem = &domain.WorkItem{
		ID:     "t-overflow",
		Title:  "Test Overflow Behavior",
		Phases: phases,
	}
	model.state = safety.NewState()
	model.state.Iteration = 10

	// Set dimensions to trigger View() rendering with height constraint
	model.width = 120
	model.height = 30

	sidebarWidth := max(45, min(60, model.width*40/100))
	contentHeight := model.height - 3

	// Render sidebar directly to verify height constraint
	sidebar := model.renderSidebar(sidebarWidth-4, contentHeight-2)
	sidebarHeight := lipgloss.Height(sidebar)

	// Sidebar content must fit within the allocated height
	require.LessOrEqual(t, sidebarHeight, contentHeight-2,
		"sidebar (%d lines) should fit in allocated height (%d lines)", sidebarHeight, contentHeight-2)

	// Progress section should be visible (top not cut off by height constraint)
	require.Contains(t, sidebar, "Progress", "Progress section should be visible in height-constrained view")
	require.Contains(t, sidebar, "Stagnation", "Stagnation should be visible in height-constrained view")
	// Overflow indicators should appear
	require.Contains(t, sidebar, "more", "should show overflow indicator")
}

func TestModelRenderSidebarCompleteStateWithManyPhases(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 50, StagnationLimit: 3})

	// Create 30 phases, all completed
	phases := make([]domain.Phase, 30)
	for i := range phases {
		phases[i] = domain.Phase{
			Name:      fmt.Sprintf("Phase %d: Task description", i+1),
			Completed: true,
		}
	}

	model.workItem = &domain.WorkItem{
		ID:     "t-complete",
		Title:  "Test Complete State",
		Phases: phases,
	}
	model.state = safety.NewState()
	model.state.Iteration = 10
	model.runState = stateComplete
	model.result = &loop.Result{ExitReason: safety.ExitReasonComplete, ExitMessage: "All phases completed"}
	model.hideTips = true // Hide tips to simplify calculation

	// Use a height that can fit fixed sections + footer + some phases
	// Fixed sections ~16 lines + footer ~5 lines = 21 lines minimum
	model.width = 120
	model.height = 35

	sidebarWidth := max(45, min(60, model.width*40/100))
	contentHeight := model.height - 3

	// Render sidebar directly to verify height constraint
	sidebar := model.renderSidebar(sidebarWidth-4, contentHeight-2)
	sidebarHeight := lipgloss.Height(sidebar)

	// Sidebar content must fit within the allocated height
	allocatedHeight := contentHeight - 2
	require.LessOrEqual(t, sidebarHeight, allocatedHeight,
		"sidebar (%d lines) should fit in allocated height (%d lines)", sidebarHeight, allocatedHeight)

	// Top sections must remain visible even with footer and done block
	require.Contains(t, sidebar, "Progress", "Progress section should be visible")
	require.Contains(t, sidebar, "Stagnation", "Stagnation should be visible")
	// Footer should be visible in complete state
	require.Contains(t, sidebar, "Exit", "Exit reason should be visible in complete state")
	// Overflow indicator should appear since we have 30 phases
	require.Contains(t, sidebar, "more", "should show overflow indicator for many phases")
}

func TestRenderPhasesContentFitsAvailableSpace(t *testing.T) {
	model := NewModel(safety.Config{MaxIterations: 50, StagnationLimit: 3})

	// Create many phases with current in the middle to trigger both overflow indicators
	phases := make([]domain.Phase, 30)
	for i := range phases {
		phases[i] = domain.Phase{
			Name:      fmt.Sprintf("Phase %d", i+1),
			Completed: i < 15,
		}
	}

	model.workItem = &domain.WorkItem{
		ID:     "t-fit",
		Title:  "Test Fit",
		Phases: phases,
	}

	tests := []struct {
		name              string
		availableHeight   int
		runState          runState
		wantMaxLines      int
		wantUpIndicator   bool
		wantDownIndicator bool
	}{
		{
			name:              "minimal height with overflow",
			availableHeight:   5,
			runState:          stateRunning,
			wantMaxLines:      5,
			wantUpIndicator:   true,
			wantDownIndicator: true,
		},
		{
			name:              "minimal height complete state",
			availableHeight:   7,
			runState:          stateComplete,
			wantMaxLines:      7,
			wantUpIndicator:   true,
			wantDownIndicator: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model.runState = tc.runState

			// Call renderPhasesContent with minimal available space
			// usedLines=0 and tipsLines=0 to isolate phases content
			content := model.renderPhasesContent(40, tc.availableHeight+3, 0, 0)

			lines := lipgloss.Height(content)
			require.LessOrEqual(t, lines, tc.wantMaxLines,
				"phases content (%d lines) should fit in available space (%d lines):\n%s",
				lines, tc.wantMaxLines, content)

			if tc.wantUpIndicator {
				require.Contains(t, content, "↑", "should have up overflow indicator")
			}
			if tc.wantDownIndicator {
				require.Contains(t, content, "↓", "should have down overflow indicator")
			}
		})
	}
}

func TestRenderSidebarTips(t *testing.T) {
	require.Len(t, tipSnippets, len(sidebarTips), "tipSnippets must match sidebarTips length")

	tests := []struct {
		name     string
		tipIndex int
		wantIdx  int
	}{
		{
			name:     "tipIndex 0 shows first tip",
			tipIndex: 0,
			wantIdx:  0,
		},
		{
			name:     "tipIndex 3 shows fourth tip",
			tipIndex: 3,
			wantIdx:  3,
		},
		{
			name:     "tipIndex wraps around",
			tipIndex: len(sidebarTips) + 2,
			wantIdx:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewModel(safety.Config{})
			model.tipIndex = tt.tipIndex

			result := model.renderSidebarTips(36)

			require.Contains(t, result, "Tips")
			require.Contains(t, result, tipSnippets[tt.wantIdx])
		})
	}
}

func TestRenderSidebarTipsHidden(t *testing.T) {
	model := NewModel(safety.Config{})
	model.hideTips = true

	result := model.renderSidebarTips(36)
	require.Empty(t, result)
}

func TestRenderSidebarTipsCycleAll(t *testing.T) {
	model := NewModel(safety.Config{})

	seen := make(map[int]bool)
	for i := range len(sidebarTips) {
		model.tipIndex = i
		result := model.renderSidebarTips(60)
		for j, snippet := range tipSnippets {
			if strings.Contains(result, snippet) {
				seen[j] = true
			}
		}
	}

	require.Len(t, seen, len(sidebarTips), "all tips should be reachable by cycling tipIndex")
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

func TestWrapLogsProgMessageWrapping(t *testing.T) {
	model := NewModel(safety.Config{})

	// Need a wide enough terminal so the viewport gets sufficient width
	// (sidebar takes ~45 cols, plus padding, so we need ~160+ for a useful viewport)
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := updatedModel.(Model)

	longMsg := protocol.MarkerProg + "This is a very long programmator message that should be word-wrapped to fit within the viewport width properly and not just keep going on a single line forever"
	m.logs = []string{longMsg}

	result := m.wrapLogs()
	require.NotEmpty(t, result)
	require.Contains(t, result, "programmator:")
	// With viewport width ~147, the long message should wrap
	require.Contains(t, result, "\n", "long PROG message should be wrapped")
}

func TestWrapLogsToolMessageWrapping(t *testing.T) {
	model := NewModel(safety.Config{})

	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := updatedModel.(Model)

	longMsg := protocol.MarkerTool + "This is a very long tool message that should be word-wrapped to fit within the viewport width properly and not overflow into a single extremely long line"
	m.logs = []string{longMsg}

	result := m.wrapLogs()
	require.NotEmpty(t, result)
	require.Contains(t, result, "\n", "long TOOL message should be wrapped")
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
	model.workItem = &domain.WorkItem{
		ID:    "t-123",
		Title: "Test",
		Phases: []domain.Phase{
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

func TestEventMsgAppended(t *testing.T) {
	model := NewModel(safety.Config{})
	model.width = 80
	model.height = 24
	model.ready = true

	updated, _ := model.Update(EventMsg{Event: event.Prog("hello from event")})
	m := updated.(Model)

	require.Len(t, m.events, 1)
	require.Equal(t, event.KindProg, m.events[0].Kind)
}

func TestRenderEventsProgAndTool(t *testing.T) {
	model := NewModel(safety.Config{})
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := updatedModel.(Model)

	m.events = []event.Event{
		event.Prog("Starting phase"),
		event.ToolUse("Read /foo/bar.go"),
		event.ToolResult("  ⎿  Read 42 lines"),
		event.Review("Running agent: quality"),
	}

	result := m.renderEvents()
	require.Contains(t, result, "programmator:")
	require.Contains(t, result, "Starting phase")
	require.Contains(t, result, "Read /foo/bar.go")
	require.Contains(t, result, "42 lines")
	require.Contains(t, result, "Running agent: quality")
}

func TestRenderEventsDiff(t *testing.T) {
	model := NewModel(safety.Config{})
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := updatedModel.(Model)

	m.events = []event.Event{
		event.DiffHunk("  ⎿  Added 2 lines"),
		event.DiffAdd("      +new line"),
		event.DiffDel("      -old line"),
		event.DiffCtx("       context"),
	}

	result := m.renderEvents()
	require.Contains(t, result, "Added 2 lines")
	require.Contains(t, result, "+new line")
	require.Contains(t, result, "-old line")
	require.Contains(t, result, "context")
}

func TestRenderEventsMarkdown(t *testing.T) {
	model := NewModel(safety.Config{})
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := updatedModel.(Model)

	m.events = []event.Event{
		event.Markdown("Some **bold** text from Claude"),
		event.Prog("Phase complete"),
	}

	result := m.renderEvents()
	require.NotEmpty(t, result)
	require.Contains(t, result, "Phase complete")
}

func TestWrapLogsUsesEventsWhenAvailable(t *testing.T) {
	model := NewModel(safety.Config{})
	updatedModel, _ := model.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	m := updatedModel.(Model)

	// Add both logs and events
	m.logs = []string{protocol.MarkerProg + "legacy log"}
	m.events = []event.Event{event.Prog("event log")}

	result := m.wrapLogs()
	// Should use events, not logs
	require.Contains(t, result, "event log")
	require.NotContains(t, result, "legacy log")
}
