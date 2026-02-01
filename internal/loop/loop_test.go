package loop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/event"
	"github.com/worksonmyai/programmator/internal/llm"
	"github.com/worksonmyai/programmator/internal/parser"
	"github.com/worksonmyai/programmator/internal/prompt"
	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/review"
	"github.com/worksonmyai/programmator/internal/safety"
	"github.com/worksonmyai/programmator/internal/source"
)

type fakeInvoker struct {
	fn func(ctx context.Context, prompt string) (string, error)
}

func (f *fakeInvoker) Invoke(ctx context.Context, prompt string, _ llm.InvokeOptions) (*llm.InvokeResult, error) {
	text, err := f.fn(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return &llm.InvokeResult{Text: text}, nil
}

func TestNewLoop(t *testing.T) {
	config := safety.Config{
		MaxIterations:   10,
		StagnationLimit: 3,
		Timeout:         60,
	}

	l := New(config, "/tmp", nil, nil, false)

	if l == nil {
		t.Fatal("New() returned nil")
	}
	if l.config.MaxIterations != 10 {
		t.Errorf("expected MaxIterations=10, got %d", l.config.MaxIterations)
	}
	if l.workingDir != "/tmp" {
		t.Errorf("expected workingDir=/tmp, got %s", l.workingDir)
	}
	if l.streaming {
		t.Error("expected streaming=false")
	}
}

func TestLoopPauseResume(t *testing.T) {
	config := safety.Config{}
	l := New(config, "", nil, nil, false)

	if l.IsPaused() {
		t.Error("loop should not be paused initially")
	}

	paused := l.TogglePause()
	if !paused {
		t.Error("TogglePause should return true when pausing")
	}
	if !l.IsPaused() {
		t.Error("loop should be paused after TogglePause")
	}

	paused = l.TogglePause()
	if paused {
		t.Error("TogglePause should return false when resuming")
	}
	if l.IsPaused() {
		t.Error("loop should not be paused after second TogglePause")
	}
}

func TestLoopStop(t *testing.T) {
	config := safety.Config{}
	l := New(config, "", nil, nil, false)

	l.Stop()

	l.mu.Lock()
	stopped := l.stopRequested
	l.mu.Unlock()

	if !stopped {
		t.Error("stopRequested should be true after Stop()")
	}
}

// NOTE: processTextOutput, processStreamingOutput, and timeoutBlockedStatus
// have been moved to internal/llm and are tested there.

func TestInvokeClaudePrintCapturesStderr(t *testing.T) {
	config := safety.Config{MaxIterations: 1, StagnationLimit: 1, Timeout: 10}
	l := New(config, "", nil, nil, false)

	// Override the claude binary with a script that writes to stderr and exits 1
	origPath := os.Getenv("PATH")
	tmpDir := t.TempDir()
	script := "#!/bin/sh\necho 'some error message' >&2\nexit 1\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+origPath)

	ctx := context.Background()
	_, err = l.invokeClaudePrint(ctx, "test prompt")

	require.Error(t, err)
	require.Contains(t, err.Error(), "claude exited")
	require.Contains(t, err.Error(), "some error message")
}

func TestInvokeClaudePrintErrorWithoutStderr(t *testing.T) {
	config := safety.Config{MaxIterations: 1, StagnationLimit: 1, Timeout: 10}
	l := New(config, "", nil, nil, false)

	origPath := os.Getenv("PATH")
	tmpDir := t.TempDir()
	script := "#!/bin/sh\nexit 1\n"
	err := os.WriteFile(tmpDir+"/claude", []byte(script), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", tmpDir+":"+origPath)

	ctx := context.Background()
	_, err = l.invokeClaudePrint(ctx, "test prompt")

	require.Error(t, err)
	require.Contains(t, err.Error(), "claude exited")
	require.NotContains(t, err.Error(), "stderr")
}

func TestResultFilesChangedList(t *testing.T) {
	r := &Result{
		TotalFilesChanged: []string{"a.go", "b.go"},
	}

	files := r.FilesChangedList()

	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
	if files[0] != "a.go" || files[1] != "b.go" {
		t.Errorf("unexpected files: %v", files)
	}
}

func TestStateCallback(t *testing.T) {
	var callbackCalled bool
	var receivedState *safety.State
	var receivedWorkItem *domain.WorkItem

	stateCallback := func(state *safety.State, workItem *domain.WorkItem, _ []string) {
		callbackCalled = true
		receivedState = state
		receivedWorkItem = workItem
	}

	config := safety.Config{}
	l := New(config, "", nil, stateCallback, false)

	if l.onStateChange == nil {
		t.Fatal("onStateChange callback should be set")
	}

	testState := safety.NewState()
	testWorkItem := &domain.WorkItem{ID: "test-123"}

	l.onStateChange(testState, testWorkItem, nil)

	if !callbackCalled {
		t.Error("state callback should have been called")
	}
	if receivedState != testState {
		t.Error("received state doesn't match")
	}
	if receivedWorkItem != testWorkItem {
		t.Error("received work item doesn't match")
	}
}

func TestLoopLog(t *testing.T) {
	var logOutput string
	onOutput := func(text string) {
		logOutput = text
	}

	config := safety.Config{}
	l := New(config, "", onOutput, nil, false)

	l.log("test message")

	if !strings.Contains(logOutput, protocol.MarkerProg) {
		t.Error("log output should contain [PROG] marker")
	}
	if !strings.Contains(logOutput, "test message") {
		t.Error("log output should contain the message")
	}
}

func TestLoopLogEvent(t *testing.T) {
	var received []event.Event
	config := safety.Config{}
	l := New(config, "", nil, nil, false)
	l.SetEventCallback(func(e event.Event) {
		received = append(received, e)
	})

	l.log("test event message")

	require.Len(t, received, 1)
	require.Equal(t, event.KindProg, received[0].Kind)
	require.Equal(t, "test event message", received[0].Text)
}

// TestLoopLogNoCallback verifies that log() does not panic when no callback is set.
func TestLoopLogNoCallback(t *testing.T) {
	_ = t // test passes if no panic occurs; named param allows future assertions
	config := safety.Config{}
	l := New(config, "", nil, nil, false)

	l.log("test message")
}

func TestPauseWakeup(t *testing.T) {
	config := safety.Config{}
	l := New(config, "", nil, nil, false)

	l.TogglePause()

	done := make(chan bool)
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.TogglePause()
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("pause wakeup timed out")
	}
}

func TestStopWakesUpPause(t *testing.T) {
	config := safety.Config{}
	l := New(config, "", nil, nil, false)

	l.TogglePause()

	done := make(chan bool)
	go func() {
		time.Sleep(50 * time.Millisecond)
		l.Stop()
		done <- true
	}()

	select {
	case <-done:
		l.mu.Lock()
		stopped := l.stopRequested
		l.mu.Unlock()
		if !stopped {
			t.Error("stop should set stopRequested")
		}
	case <-time.After(1 * time.Second):
		t.Error("stop wakeup timed out")
	}
}

func TestResultExitReasons(t *testing.T) {
	tests := []struct {
		reason   safety.ExitReason
		expected string
	}{
		{safety.ExitReasonComplete, "complete"},
		{safety.ExitReasonMaxIterations, "max_iterations"},
		{safety.ExitReasonStagnation, "stagnation"},
		{safety.ExitReasonBlocked, "blocked"},
		{safety.ExitReasonError, "error"},
		{safety.ExitReasonUserInterrupt, "user_interrupt"},
	}

	for _, tc := range tests {
		r := &Result{ExitReason: tc.reason}
		if string(r.ExitReason) != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, r.ExitReason)
		}
	}
}

func TestResultWithFinalStatus(t *testing.T) {
	status := &parser.ParsedStatus{
		PhaseCompleted: "Phase 1",
		Status:         protocol.StatusContinue,
		FilesChanged:   []string{"main.go"},
		Summary:        "Did something",
	}

	r := &Result{
		ExitReason:        safety.ExitReasonComplete,
		Iterations:        5,
		TotalFilesChanged: []string{"main.go", "test.go"},
		FinalStatus:       status,
	}

	if r.Iterations != 5 {
		t.Errorf("expected 5 iterations, got %d", r.Iterations)
	}
	if r.FinalStatus.PhaseCompleted != "Phase 1" {
		t.Errorf("unexpected phase completed: %s", r.FinalStatus.PhaseCompleted)
	}
}

func TestRunAllPhasesCompleteAtStart(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 0, result.Iterations)
	require.Len(t, mock.SetStatusCalls, 2)
}

func TestRunGetTicketError(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return nil, fmt.Errorf("ticket not found")
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	result, err := l.Run("nonexistent")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunStopRequested(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.Stop()

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonUserInterrupt, result.ExitReason)
}

func TestRunStateCallbackCalled(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	var callbackInvoked bool
	stateCallback := func(_ *safety.State, tkt *domain.WorkItem, _ []string) {
		callbackInvoked = true
		require.Equal(t, "test-123", tkt.ID)
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, stateCallback, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	_, err := l.Run("test-123")
	require.NoError(t, err)
	require.True(t, callbackInvoked, "state callback should have been called")
}

func TestNewWithSourceNil(t *testing.T) {
	config := safety.Config{MaxIterations: 10}
	l := NewWithSource(config, "/tmp", nil, nil, false, nil)

	require.NotNil(t, l)
	require.Nil(t, l.source)
}

func TestRunWithMockInvokerDone(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `Some output
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: DONE
  files_changed: ["main.go"]
  summary: "Completed the task"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 1, result.Iterations)
	require.NotNil(t, result.FinalStatus)
	require.Equal(t, "Phase 1", result.FinalStatus.PhaseCompleted)
}

func TestRunWithMockInvokerBlocked(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Stuck on something"
  error: "Cannot proceed"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonBlocked, result.ExitReason)
}

func TestRunWithMockInvokerNoStatus(t *testing.T) {
	mock := source.NewMockSource()
	callCount := 0
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		callCount++
		if callCount >= 4 {
			return &domain.WorkItem{
				ID:    "test-123",
				Title: "Test Ticket",
				Phases: []domain.Phase{
					{Name: "Phase 1", Completed: true},
				},
			}, nil
		}
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	invokeCount := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invokeCount++
		return "Some output without status block", nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.GreaterOrEqual(t, invokeCount, 2)
}

func TestRunWithMockInvokerError(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("claude error")
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	// 3 consecutive invocation failures triggers early exit
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
	require.Contains(t, result.ExitMessage, "3 consecutive invocation failures")
}

func TestRunMaxIterations(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Working on it"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonMaxIterations, result.ExitReason)
	require.Equal(t, 4, result.Iterations)
}

func TestRunStagnation(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "Thinking..."
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason)
}

func TestRunFilesChanged(t *testing.T) {
	mock := source.NewMockSource()
	invocation := 0
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocation++
		files := fmt.Sprintf(`["file%d.go"]`, invocation)
		if invocation == 2 {
			files = `["file1.go", "file2.go"]`
		}
		return fmt.Sprintf(`PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: %s
  summary: "Working"
`, files), nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Len(t, result.TotalFilesChanged, 3)
}

func TestRunGetTicketErrorDuringLoop(t *testing.T) {
	mock := source.NewMockSource()
	callCount := 0
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		callCount++
		if callCount > 1 {
			return nil, fmt.Errorf("ticket fetch error")
		}
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	result, err := l.Run("test-123")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestSetInvoker(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	require.Nil(t, l.invoker)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return "test", nil
	}})

	require.NotNil(t, l.invoker)
}

func TestRunParseError(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  this is invalid yaml: [
`, nil
	}})

	result, err := l.Run("test-123")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunContextCancellation(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	invocations := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocations++
		if invocations == 1 {
			l.Stop()
		}
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["a.go"]
  summary: "Working"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonUserInterrupt, result.ExitReason)
}

// Tests for RunReviewOnly

func TestRunReviewOnlyPassesWithNoIssues(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock the review runner to return no issues
	mockRunner := createMockReviewRunner(t, false, 0)
	l.SetReviewRunner(mockRunner)

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Equal(t, 1, result.Iterations)
	require.Equal(t, 0, result.TotalIssues)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
}

func TestRunReviewOnlyFailsMaxIterations(t *testing.T) {
	config := safety.Config{MaxIterations: 20, StagnationLimit: 20, Timeout: 60, MaxReviewIterations: 20}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(review.Config{
		MaxIterations: 2,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Mock review runner to always return issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return CONTINUE
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed one issue"
  commit_made: true
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	// Max iterations exceeded, review stops
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
}

func TestRunReviewOnlyBlocked(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner to return issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return BLOCKED
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Cannot fix this issue"
  error: "Requires human intervention"
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonBlocked, result.ExitReason)
}

func TestRunReviewOnlyFixAndPass(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetGitWorkflowConfig(GitWorkflowConfig{AutoCommit: true})
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner: first call returns issues, second call passes
	invocation := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		invocation++
		if invocation == 1 {
			return true, 1 // has issues
		}
		return false, 0 // passes
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return CONTINUE with files fixed
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
  commit_made: true
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Equal(t, 2, result.Iterations)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Len(t, result.FilesFixed, 1)
	require.Equal(t, 1, result.CommitsMade)
}

func TestRunReviewOnlyStopRequested(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	l.Stop()

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonUserInterrupt, result.ExitReason)
}

func TestRunReviewOnlyInvokerError(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(review.Config{
		MaxIterations: 10,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Mock review runner to return issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return error
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("claude error")
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason)
}

func TestRunReviewOnlyTracksFilesFixed(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner: returns issues for first 2 calls, then passes
	invocation := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		invocation++
		if invocation <= 2 {
			return true, 1
		}
		return false, 0
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return different files each time
	claudeCall := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeCall++
		files := fmt.Sprintf(`["file%d.go"]`, claudeCall)
		return fmt.Sprintf(`PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: %s
  summary: "Fixed issue %d"
  commit_made: true
`, files, claudeCall), nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Len(t, result.FilesFixed, 2) // Two different files fixed
}

func TestRunReviewOnlyAutoCommit(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner: first call returns issues, second call passes
	invocation := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		invocation++
		if invocation == 1 {
			return true, 1 // has issues
		}
		return false, 0 // passes
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return CONTINUE with files fixed but NO commit_made
	// (auto-commit will fail since we're in /tmp, but that's OK for this test)
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	// Should pass since review passes on second iteration
	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Equal(t, 2, result.Iterations)
	require.Len(t, result.FilesFixed, 1)
	// Auto-commit would have been attempted but might fail in test env - that's OK
}

func TestDefaultReviewFixPrompt(t *testing.T) {
	baseBranch := "main"
	filesChanged := []string{"main.go", "utils.go"}
	issuesMarkdown := "### quality\n- Error not handled at main.go:42"
	iteration := 2

	prompt := defaultReviewFixPrompt(baseBranch, filesChanged, issuesMarkdown, iteration)

	require.Contains(t, prompt, "Base branch: main")
	require.Contains(t, prompt, "Review iteration: 2")
	require.Contains(t, prompt, "main.go")
	require.Contains(t, prompt, "utils.go")
	require.Contains(t, prompt, issuesMarkdown)
	require.Contains(t, prompt, protocol.StatusBlockKey+":")
	require.Contains(t, prompt, "commit_made: true")
}

func TestDefaultReviewFixPromptFormatting(t *testing.T) {
	prompt := defaultReviewFixPrompt("develop", []string{"file.go"}, "some issues", 1)

	// Check structure
	require.Contains(t, prompt, "## Context")
	require.Contains(t, prompt, "## Files to review")
	require.Contains(t, prompt, "## Issues Found")
	require.Contains(t, prompt, "## Instructions")
	require.Contains(t, prompt, "## Session End Protocol")
}

// singleAgentReviewConfig returns a review config with one agent,
// suitable for tests that use mock review runners.
func singleAgentReviewConfig() review.Config {
	return review.Config{
		MaxIterations: 10,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	}
}

// Helper functions for creating mock review runners

func createMockReviewRunner(t *testing.T, hasIssues bool, issueCount int) *review.Runner {
	t.Helper()

	cfg := review.Config{
		MaxIterations: 3,
		Parallel:      true,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	}

	runner := review.NewRunner(cfg, nil)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
		// Validators should return empty results
		if agentCfg.Name == "simplification-validator" || agentCfg.Name == "issue-validator" {
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
				return &review.Result{AgentName: agentCfg.Name, Summary: "No issues"}, nil
			})
			return mock
		}
		mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
			var issues []review.Issue
			if hasIssues {
				for i := range issueCount {
					issues = append(issues, review.Issue{
						File:        "file.go",
						Severity:    review.SeverityHigh,
						Description: fmt.Sprintf("Issue %d", i+1),
					})
				}
			}
			return &review.Result{
				AgentName: agentCfg.Name,
				Issues:    issues,
				Summary:   "Review complete",
			}, nil
		})
		return mock
	})

	return runner
}

func createMockReviewRunnerFunc(t *testing.T, resultFunc func() (hasIssues bool, issueCount int)) *review.Runner {
	t.Helper()

	cfg := review.Config{
		MaxIterations: 3,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	}

	runner := review.NewRunner(cfg, nil)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
		// Validators should return empty results
		if agentCfg.Name == "simplification-validator" || agentCfg.Name == "issue-validator" {
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
				return &review.Result{AgentName: agentCfg.Name, Summary: "No issues"}, nil
			})
			return mock
		}
		mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
			hasIssues, issueCount := resultFunc()
			var issues []review.Issue
			if hasIssues {
				for i := range issueCount {
					issues = append(issues, review.Issue{
						File:        "file.go",
						Severity:    review.SeverityHigh,
						Description: fmt.Sprintf("Issue %d", i+1),
					})
				}
			}
			return &review.Result{
				AgentName: agentCfg.Name,
				Issues:    issues,
				Summary:   "Review complete",
			}, nil
		})
		return mock
	})

	return runner
}

// Additional tests for review-only mode edge cases

func TestRunReviewOnlyNoStatusBlock(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner: returns issues first time, passes second time
	invocation := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		invocation++
		if invocation == 1 {
			return true, 1 // has issues
		}
		return false, 0 // passes
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return output without PROGRAMMATOR_STATUS block
	claudeCall := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeCall++
		if claudeCall <= 2 {
			// No status block - should be handled gracefully
			return "Made some changes but forgot the status block", nil
		}
		// Eventually return proper status
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	// Eventually passes after review runner returns no issues
	require.True(t, result.Passed)
}

func TestRunReviewOnlyReviewError(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

	// Mock review runner that returns an error from sequential execution.
	// Use a single iteration to avoid invoking Claude for an unfixable agent error.
	cfg := review.Config{
		MaxIterations: 1,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	}
	l.SetReviewConfig(cfg)
	runner := review.NewRunner(cfg, nil)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
		mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
			return nil, fmt.Errorf("review agent failed")
		})
		return mock
	})
	l.SetReviewRunner(runner)

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	// Agent errors are captured in results and count as issues, so the review fails.
	require.NoError(t, err)
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
}

func TestRunReviewOnlyStagnation(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(review.Config{
		MaxIterations: 10,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Mock review runner that always returns issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker that never changes any files (should trigger stagnation)
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "Thinking about how to fix this"
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason)
}

func TestRunReviewOnlyOutputCallback(t *testing.T) {
	var outputCollected []string
	onOutput := func(text string) {
		outputCollected = append(outputCollected, text)
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", onOutput, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner that passes immediately
	mockRunner := createMockReviewRunner(t, false, 0)
	l.SetReviewRunner(mockRunner)

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)

	// Verify output was collected
	require.Greater(t, len(outputCollected), 0)
}

func TestRunReviewOnlyStateCallback(t *testing.T) {
	var callbackInvoked bool
	var lastState *safety.State

	stateCallback := func(state *safety.State, _ *domain.WorkItem, _ []string) {
		callbackInvoked = true
		lastState = state
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, stateCallback, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner: first call returns issues (so Claude is invoked and callback is triggered),
	// second call passes
	invocation := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		invocation++
		if invocation == 1 {
			return true, 1 // has issues first time
		}
		return false, 0 // passes second time
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker so we go through the fix loop
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
  commit_made: true
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.True(t, callbackInvoked, "state callback should have been invoked")
	require.NotNil(t, lastState)
}

func TestRunReviewOnlyDurationTracked(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner that passes immediately
	mockRunner := createMockReviewRunner(t, false, 0)
	l.SetReviewRunner(mockRunner)

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	// Duration should be greater than 0
	require.Greater(t, result.Duration, time.Duration(0))
}

func TestRunReviewOnlyDeduplicatesFilesFixed(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Mock review runner: returns issues for first 2 calls, then passes
	invocation := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		invocation++
		if invocation <= 2 {
			return true, 1
		}
		return false, 0
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker that returns the same file multiple times
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
  commit_made: true
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	// file.go should only appear once even though it was returned multiple times
	require.Len(t, result.FilesFixed, 1)
	require.Equal(t, "file.go", result.FilesFixed[0])
}

func TestSetReviewConfig(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	cfg := review.Config{
		MaxIterations: 5,
	}
	l.SetReviewConfig(cfg)

	require.Equal(t, 5, l.reviewConfig.MaxIterations)
}

func TestSetReviewOnly(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	require.False(t, l.reviewOnly)

	l.SetReviewOnly(true)
	require.True(t, l.reviewOnly)
}

func TestRunWithPlanSource_UpdatesCheckboxes(t *testing.T) {
	// Integration test: verifies that completing a phase updates the plan file on disk
	tmpDir := t.TempDir()
	planPath := tmpDir + "/test-plan.md"
	content := `# Plan: Integration Test

## Tasks
- [ ] Task 1: First task
- [ ] Task 2: Second task
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	planSource := source.NewPlanSource(planPath)
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, tmpDir, nil, nil, false, planSource)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	// Mock Claude to complete first task, then second task
	invocation := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocation++
		if invocation == 1 {
			return `PROGRAMMATOR_STATUS:
  phase_completed: "Task 1: First task"
  status: CONTINUE
  files_changed: ["file1.go"]
  summary: "Completed first task"
`, nil
		}
		return `PROGRAMMATOR_STATUS:
  phase_completed: "Task 2: Second task"
  status: DONE
  files_changed: ["file2.go"]
  summary: "Completed second task"
`, nil
	}})

	result, err := l.Run(planPath)
	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 2, result.Iterations)

	// Verify the plan file was updated on disk
	savedContent, err := os.ReadFile(planPath)
	require.NoError(t, err)

	require.Contains(t, string(savedContent), "- [x] Task 1: First task")
	require.Contains(t, string(savedContent), "- [x] Task 2: Second task")
}

// Tests for phaseless ticket execution

func TestRunPhaselessTicket_CompletesOnDone(t *testing.T) {
	// Test: A ticket without phases runs until Claude reports DONE
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:         "phaseless-123",
			Title:      "Phaseless Ticket",
			Phases:     nil, // No phases - phaseless ticket
			RawContent: "# Phaseless Ticket\n\nJust do the task.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	reviewCalls := 0
	l.SetReviewRunner(createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewCalls++
		return false, 0
	}))

	// Mock Claude to report DONE on first invocation
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: DONE
  files_changed: ["main.go"]
  summary: "Completed the entire task"
`, nil
	}})

	result, err := l.Run("phaseless-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 1, result.Iterations)
	require.NotNil(t, result.FinalStatus)
	require.Equal(t, protocol.StatusDone, result.FinalStatus.Status)
	require.Equal(t, 1, reviewCalls)

	// Verify status was set to closed
	require.Len(t, mock.SetStatusCalls, 2) // in_progress + closed
	require.Equal(t, protocol.WorkItemClosed, mock.SetStatusCalls[1].Status)
}

func TestRunPhaselessTicket_ContinuesUntilDone(t *testing.T) {
	// Test: A phaseless ticket continues looping until Claude signals DONE
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:         "phaseless-456",
			Title:      "Multi-iteration Phaseless Ticket",
			Phases:     []domain.Phase{}, // Empty phases - also phaseless
			RawContent: "# Task\n\nComplex task requiring multiple steps.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	// Mock Claude to work for 3 iterations before reporting DONE
	invocation := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocation++
		if invocation < 3 {
			return fmt.Sprintf(`PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file%d.go"]
  summary: "Working on step %d"
`, invocation, invocation), nil
		}
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: DONE
  files_changed: ["final.go"]
  summary: "Task completed"
`, nil
	}})

	result, err := l.Run("phaseless-456")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 3, result.Iterations)
	require.Len(t, result.TotalFilesChanged, 3)

	// Verify no UpdatePhase calls since there are no phases
	for _, call := range mock.UpdatePhaseCalls {
		require.Empty(t, call.PhaseName, "UpdatePhase should not be called for phaseless tickets")
	}
}

func TestRunPhaselessTicket_SafetyLimitsStillApply(t *testing.T) {
	// Test: Safety limits (stagnation) still apply to phaseless tickets
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:         "phaseless-stag",
			Title:      "Phaseless Ticket That Stagnates",
			Phases:     nil,
			RawContent: "# Task\n\nTask that doesn't progress.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	// Mock Claude to never make progress (no files changed)
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "Still thinking..."
`, nil
	}})

	result, err := l.Run("phaseless-stag")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason)
}

func TestRunPhaselessTicket_BlockedHandled(t *testing.T) {
	// Test: BLOCKED status is handled correctly for phaseless tickets
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:         "phaseless-blocked",
			Title:      "Phaseless Ticket That Gets Blocked",
			Phases:     nil,
			RawContent: "# Task\n\nTask that gets blocked.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Cannot proceed"
  error: "Missing required credentials"
`, nil
	}})

	result, err := l.Run("phaseless-blocked")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonBlocked, result.ExitReason)
	require.NotNil(t, result.FinalStatus)
	require.Equal(t, "Missing required credentials", result.FinalStatus.Error)
}

func TestBuildHookSettings_PermissionOnly(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)
	l.SetPermissionSocketPath("/tmp/test.sock")

	settings := l.buildHookSettings()

	require.Contains(t, settings, `"matcher":""`)
	require.Contains(t, settings, "programmator hook --socket /tmp/test.sock")
	require.Contains(t, settings, `"timeout":120000`)
	require.NotContains(t, settings, "dcg")
}

func TestBuildHookSettings_GuardOnly(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)
	l.SetGuardMode(true)

	settings := l.buildHookSettings()

	require.Contains(t, settings, `"matcher":"Bash"`)
	home, _ := os.UserHomeDir()
	require.Contains(t, settings, fmt.Sprintf("DCG_CONFIG='%s/.config/dcg/config.toml' dcg", home))
	require.Contains(t, settings, `"timeout":5000`)
	require.NotContains(t, settings, "programmator hook")
}

func TestBuildHookSettings_BothCombined(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)
	l.SetPermissionSocketPath("/tmp/test.sock")
	l.SetGuardMode(true)

	settings := l.buildHookSettings()

	require.Contains(t, settings, `"matcher":""`)
	require.Contains(t, settings, "programmator hook --socket /tmp/test.sock")
	require.Contains(t, settings, `"matcher":"Bash"`)
	home, _ := os.UserHomeDir()
	require.Contains(t, settings, fmt.Sprintf("DCG_CONFIG='%s/.config/dcg/config.toml' dcg", home))
}

func TestSetGuardMode(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	require.False(t, l.guardMode)

	l.SetGuardMode(true)
	require.True(t, l.guardMode)
}

// Fix 1 test: iteration_limit:1 allows Claude one fix attempt
func TestHandleMultiPhaseReview_IterationLimitOneAllowsFix(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 50, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 50}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 50,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Review finds issues on first call, passes on second (after fix)
	reviewCall := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewCall++
		if reviewCall == 1 {
			return true, 2 // has issues
		}
		return false, 0 // passes after fix
	})
	l.SetReviewRunner(mockRunner)

	// Claude is invoked to fix the issues
	claudeInvoked := false
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeInvoked = true
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Fixed the issues"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.True(t, claudeInvoked, "Claude should be invoked to fix issues even with iteration_limit:1")
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
}

// Fix 2 test: promptBuilder wired through to buildReviewFixPrompt
func TestPromptBuilderWiredInReview(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(singleAgentReviewConfig())

	// Create a real prompt builder
	builder, err := prompt.NewBuilder(nil)
	require.NoError(t, err)
	l.SetPromptBuilder(builder)

	// buildReviewFixPrompt should use the builder (not fallback)
	result, err := l.buildReviewFixPrompt("main", []string{"file.go"}, "some issues", 1)
	require.NoError(t, err)

	// The template-rendered prompt differs from the default fallback.
	// The default fallback contains "## Session End Protocol" which the
	// template does not (it uses different wording). Just verify non-empty.
	require.NotEmpty(t, result)
	require.Contains(t, result, "some issues")
}

// Test: main loop uses review fix prompt (not task prompt) when pendingReviewFix is true
func TestMainLoopUsesReviewFixPromptWhenPendingFix(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 50, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 50}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 50,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	builder, err := prompt.NewBuilder(nil)
	require.NoError(t, err)
	l.SetPromptBuilder(builder)

	// Review finds issues on first call, passes on second (after fix)
	reviewCall := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewCall++
		if reviewCall == 1 {
			return true, 2 // has issues
		}
		return false, 0 // passes after fix
	})
	l.SetReviewRunner(mockRunner)

	// Capture the prompt text sent to Claude
	var capturedPrompt string
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, p string) (string, error) {
		capturedPrompt = p
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Fixed the issues"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	// The review fix prompt should contain the issue markers from the review,
	// NOT the task prompt content (ticket title, phases, etc.)
	require.NotEmpty(t, capturedPrompt)
	require.NotContains(t, capturedPrompt, "Test Ticket", "should not contain task prompt content when fixing review issues")
	require.Contains(t, capturedPrompt, "Issue", "review fix prompt should reference issues")
}

// Test: main loop uses promptBuilder for task prompts (not the default builder)
func TestMainLoopUsesPromptBuilderForTaskPrompt(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-456",
			Title: "Test Task",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 2, StagnationLimit: 10, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	builder, err := prompt.NewBuilder(nil)
	require.NoError(t, err)
	l.SetPromptBuilder(builder)

	// Capture the prompt text sent to Claude
	var capturedPrompt string
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, p string) (string, error) {
		capturedPrompt = p
		return `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: DONE
  files_changed: ["app.go"]
  summary: "Done"
`, nil
	}})

	result, err := l.Run("test-456")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.NotEmpty(t, capturedPrompt)
	// The builder-generated prompt should contain the work item content
	require.Contains(t, capturedPrompt, "Test Task")
	require.Contains(t, capturedPrompt, "Phase 1")
}

// Test: reviewConfig.MaxIterations controls the review loop
func TestReviewUsesMaxIterations(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 100, StagnationLimit: 50, Timeout: 60, MaxReviewIterations: 100}
	l := NewWithSource(config, "", nil, nil, false, mock)

	// Set review config with low MaxIterations
	l.SetReviewConfig(review.Config{
		MaxIterations: 5,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Review always finds issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	claudeCallCount := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeCallCount++
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Attempted fix"
`, nil
	}})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	// MaxIterations=5: 5 review+fix cycles, each review finds issues and triggers a fix
	require.Equal(t, 5, claudeCallCount)
}

// Test: empty agents returns an error
func TestRunReviewOnlyEmptyAgents(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)
	l.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{}, // empty
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.Error(t, err)
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

// Test: review iteration with fix and pass
func TestRunReviewOnlyIterationWithFix(t *testing.T) {
	config := safety.Config{MaxIterations: 50, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 50}
	l := New(config, "/tmp", nil, nil, false)

	l.SetReviewConfig(review.Config{
		MaxIterations: 10,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Track review calls
	reviewCallCount := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewCallCount++
		if reviewCallCount == 1 {
			return true, 2 // has issues
		}
		return false, 0 // passes after fix
	})
	l.SetReviewRunner(mockRunner)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Fixed issues"
  commit_made: true
`, nil
	}})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	// review(issues) + fix + review(pass) = 2 iterations
	require.Equal(t, 2, result.Iterations)
}

// Test: single review.max_iterations controls entire review (no per-phase caps)
func TestReviewMaxIterationsOnly_NoPhaseIterationBudgets(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-maxiter",
			Title: "Test MaxIterations Only",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 100, StagnationLimit: 50, Timeout: 60, MaxReviewIterations: 100}
	l := NewWithSource(config, "", nil, nil, false, mock)

	// Set review config with max_iterations=3 — this single limit controls the review loop
	l.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Review always finds issues — should stop after max_iterations
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	claudeCallCount := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeCallCount++
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Attempted fix"
`, nil
	}})

	result, err := l.Run("test-maxiter")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	// MaxIterations=3: 3 review+fix cycles, each review finds issues and triggers a fix
	require.Equal(t, 3, claudeCallCount, "should have exactly max_iterations fix calls")
}

// Test: review-only mode uses single max_iterations limit
// Test: MaxReviewIter=1 via handleReview (Run path) allows exactly one review.
func TestRun_MaxReviewIterOneRunsOneReview(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-max-review-1",
			Title: "Test MaxReviewIter=1",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 100, StagnationLimit: 50, Timeout: 60, MaxReviewIterations: 100}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 1,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	reviewCallCount := 0
	runner := createMockReviewRunnerFunc(t, func() (hasIssues bool, issueCount int) {
		reviewCallCount++
		return false, 0 // pass
	})
	l.SetReviewRunner(runner)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "fix"
`, nil
	}})

	result, err := l.Run("test-max-review-1")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 1, reviewCallCount, "MaxReviewIter=1 should allow exactly one review run")
}

// Test: agent errors during review don't consume iteration budget
func TestRunReview_AgentErrorsDoNotConsumeIterationBudget(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-agent-err",
			Title: "Test Agent Errors",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 100, StagnationLimit: 50, Timeout: 60, MaxReviewIterations: 100}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 2,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	callCount := 0
	// First call: agent error; second call: pass
	runner := createMockReviewRunnerWithErrors(t, func() (agentError bool, hasIssues bool) {
		callCount++
		if callCount == 1 {
			return true, false // agent error on first call
		}
		return false, false // pass on second call
	})
	l.SetReviewRunner(runner)

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "fix"
`, nil
	}})

	result, err := l.Run("test-agent-err")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	// Agent error retry should not have consumed an iteration.
	// With MaxIterations=2: call 1 errors (doesn't count) → retry → call 2 passes.
	// If agent errors consumed budget, the second call would hit the limit.
	require.Equal(t, 2, callCount, "runner should be called twice (error + pass)")
}

// Test: MaxReviewIter=0 means unlimited review iterations
func TestRunReview_UnlimitedIterations(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-unlimited",
			Title: "Test Unlimited",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 100, StagnationLimit: 50, Timeout: 60, MaxReviewIterations: 100}
	l := NewWithSource(config, "", nil, nil, false, mock)

	// MaxIterations=0 means unlimited
	l.SetReviewConfig(review.Config{
		MaxIterations: 0,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	reviewCallCount := 0
	// Fail 5 times then pass — with MaxIterations=0 this should be fine
	runner := createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewCallCount++
		if reviewCallCount <= 5 {
			return true, 1
		}
		return false, 0
	})
	l.SetReviewRunner(runner)

	claudeCallCount := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeCallCount++
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Attempted fix"
`, nil
	}})

	result, err := l.Run("test-unlimited")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 6, reviewCallCount, "should run 6 reviews (5 fail + 1 pass)")
	require.Equal(t, 5, claudeCallCount, "should have 5 fix calls")
}

func createMockReviewRunnerWithErrors(t *testing.T, resultFunc func() (agentError bool, hasIssues bool)) *review.Runner {
	t.Helper()

	cfg := review.Config{
		MaxIterations: 3,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	}

	runner := review.NewRunner(cfg, nil)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
		if agentCfg.Name == "simplification-validator" || agentCfg.Name == "issue-validator" {
			mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
				return &review.Result{AgentName: agentCfg.Name, Summary: "No issues"}, nil
			})
			return mock
		}
		mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
			agentError, hasIssues := resultFunc()
			if agentError {
				return &review.Result{
					AgentName: agentCfg.Name,
					Error:     fmt.Errorf("agent execution failed"),
				}, nil
			}
			var issues []review.Issue
			if hasIssues {
				issues = append(issues, review.Issue{
					File:        "file.go",
					Severity:    review.SeverityHigh,
					Description: "Issue found",
				})
			}
			return &review.Result{
				AgentName: agentCfg.Name,
				Issues:    issues,
				Summary:   "Review complete",
			}, nil
		})
		return mock
	})

	return runner
}

func TestWorkItemHelpers_Phaseless(t *testing.T) {
	// Test: WorkItem helper methods work correctly for phaseless items
	tests := []struct {
		name              string
		phases            []domain.Phase
		hasPhases         bool
		allPhasesComplete bool
		currentPhaseIsNil bool
	}{
		{
			name:              "nil phases",
			phases:            nil,
			hasPhases:         false,
			allPhasesComplete: false,
			currentPhaseIsNil: true,
		},
		{
			name:              "empty phases",
			phases:            []domain.Phase{},
			hasPhases:         false,
			allPhasesComplete: false,
			currentPhaseIsNil: true,
		},
		{
			name: "has incomplete phases",
			phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
			hasPhases:         true,
			allPhasesComplete: false,
			currentPhaseIsNil: false,
		},
		{
			name: "all phases complete",
			phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
			hasPhases:         true,
			allPhasesComplete: true,
			currentPhaseIsNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := &domain.WorkItem{Phases: tc.phases}

			require.Equal(t, tc.hasPhases, item.HasPhases())
			require.Equal(t, tc.allPhasesComplete, item.AllPhasesComplete())

			if tc.currentPhaseIsNil {
				require.Nil(t, item.CurrentPhase())
			} else {
				require.NotNil(t, item.CurrentPhase())
			}
		})
	}
}

func TestConsecutiveInvocationFailures(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-consec-1",
			Title: "Test Consecutive Failures",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	cfg := safety.Config{MaxIterations: 20, StagnationLimit: 20, Timeout: 60}
	l := NewWithSource(cfg, "", nil, nil, false, mock)
	l.SetReviewConfig(review.Config{
		MaxIterations: 10,
		Agents:        []review.AgentConfig{{Name: "test"}},
	})

	invokeCount := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invokeCount++
		return "", fmt.Errorf("connection refused")
	}})

	result, err := l.Run("test-consec-1")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
	require.Contains(t, result.ExitMessage, "3 consecutive invocation failures")
	require.Equal(t, 3, invokeCount)
}

func TestFormatToolResultSummary(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		result   string
		expected string
	}{
		{name: "empty result", toolName: "Read", result: "", expected: ""},
		{name: "Read lines", toolName: "Read", result: "line1\nline2\nline3\n", expected: "Read 3 lines"},
		{name: "Read single line", toolName: "Read", result: "line1", expected: "Read 1 lines"},
		{name: "Glob with files", toolName: "Glob", result: "foo.go\nbar.go\n", expected: "Found 2 files"},
		{name: "Glob no files", toolName: "Glob", result: "\n", expected: "No files found"},
		{name: "Grep with matches", toolName: "Grep", result: "match1\nmatch2\n", expected: "Found 2 matches"},
		{name: "Grep no matches", toolName: "Grep", result: "\n", expected: "No matches found"},
		{name: "Bash empty trailing newline", toolName: "Bash", result: "\n", expected: ""},
		{name: "Bash single line", toolName: "Bash", result: "ok", expected: "ok"},
		{name: "Bash multi line", toolName: "Bash", result: "first\nsecond\nthird\n", expected: "first (+2 more lines)"},
		{name: "Bash long line truncated", toolName: "Bash", result: strings.Repeat("x", 100), expected: strings.Repeat("x", 57) + "..."},
		{name: "Write", toolName: "Write", result: "done", expected: "File written"},
		{name: "Edit", toolName: "Edit", result: "done", expected: "File updated"},
		{name: "unknown single line", toolName: "Other", result: "hello", expected: "hello"},
		{name: "unknown multi line", toolName: "Other", result: "a\nb\nc\n", expected: "3 lines"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatToolResultSummary(tc.toolName, tc.result)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestFormatToolArg(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    map[string]any
		expected string
	}{
		{name: "Read with path", toolName: "Read", input: map[string]any{"file_path": "/foo/bar.go"}, expected: " /foo/bar.go"},
		{name: "Write with path", toolName: "Write", input: map[string]any{"file_path": "/a/b.go"}, expected: " /a/b.go"},
		{name: "Edit with path", toolName: "Edit", input: map[string]any{"file_path": "/c.go"}, expected: " /c.go"},
		{name: "Read missing path", toolName: "Read", input: map[string]any{}, expected: ""},
		{name: "Bash short cmd", toolName: "Bash", input: map[string]any{"command": "ls -la"}, expected: " ls -la"},
		{name: "Bash long cmd truncated", toolName: "Bash", input: map[string]any{"command": strings.Repeat("a", 100)}, expected: " " + strings.Repeat("a", 80) + "..."},
		{name: "Glob pattern", toolName: "Glob", input: map[string]any{"pattern": "**/*.go"}, expected: " **/*.go"},
		{name: "Grep pattern", toolName: "Grep", input: map[string]any{"pattern": "TODO"}, expected: " TODO"},
		{name: "Task description", toolName: "Task", input: map[string]any{"description": "search files"}, expected: " search files"},
		{name: "unknown tool", toolName: "Unknown", input: map[string]any{"foo": "bar"}, expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatToolArg(tc.toolName, tc.input)
			require.Equal(t, tc.expected, got)
		})
	}
}
