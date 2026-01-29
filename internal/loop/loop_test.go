package loop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/parser"
	"github.com/worksonmyai/programmator/internal/review"
	"github.com/worksonmyai/programmator/internal/safety"
	"github.com/worksonmyai/programmator/internal/source"
)

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

func TestProcessTextOutput(t *testing.T) {
	var collected []string
	onOutput := func(text string) {
		collected = append(collected, text)
	}

	config := safety.Config{}
	l := New(config, "", onOutput, nil, false)

	input := "line1\nline2\nline3"
	reader := strings.NewReader(input)

	output := l.processTextOutput(reader)

	expected := "line1\nline2\nline3\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}

	if len(collected) != 3 {
		t.Errorf("expected 3 callbacks, got %d", len(collected))
	}
}

func TestProcessStreamingOutput(t *testing.T) {
	var collected []string
	onOutput := func(text string) {
		collected = append(collected, text)
	}

	config := safety.Config{}
	l := New(config, "", onOutput, nil, true)

	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":" World"}]}}
{"type":"result","result":"final"}`

	reader := strings.NewReader(input)
	output := l.processStreamingOutput(reader)

	if output != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", output)
	}

	if len(collected) != 2 {
		t.Errorf("expected 2 callbacks, got %d", len(collected))
	}
}

func TestProcessStreamingOutputEmpty(t *testing.T) {
	config := safety.Config{}
	l := New(config, "", nil, nil, true)

	input := `{"type":"result","result":"only result"}`
	reader := strings.NewReader(input)

	output := l.processStreamingOutput(reader)

	if output != "only result" {
		t.Errorf("expected 'only result', got %q", output)
	}
}

func TestTimeoutBlockedStatus(t *testing.T) {
	status := timeoutBlockedStatus()

	if !strings.Contains(status, "PROGRAMMATOR_STATUS") {
		t.Error("timeout status should contain PROGRAMMATOR_STATUS")
	}
	if !strings.Contains(status, "BLOCKED") {
		t.Error("timeout status should contain BLOCKED")
	}
	if !strings.Contains(status, "timed out") {
		t.Error("timeout status should contain timed out message")
	}
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
	var receivedWorkItem *source.WorkItem

	stateCallback := func(state *safety.State, workItem *source.WorkItem, _ []string) {
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
	testWorkItem := &source.WorkItem{ID: "test-123"}

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

	if !strings.Contains(logOutput, "[PROG]") {
		t.Error("log output should contain [PROG] marker")
	}
	if !strings.Contains(logOutput, "test message") {
		t.Error("log output should contain the message")
	}
}

func TestLoopLogNoCallback(_ *testing.T) {
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
		Status:         parser.StatusContinue,
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
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 0, result.Iterations)
	require.Len(t, mock.SetStatusCalls, 2)
}

func TestRunGetTicketError(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
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
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
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
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	var callbackInvoked bool
	stateCallback := func(_ *safety.State, tkt *source.WorkItem, _ []string) {
		callbackInvoked = true
		require.Equal(t, "test-123", tkt.ID)
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, stateCallback, false, mock)

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
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `Some output
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: DONE
  files_changed: ["main.go"]
  summary: "Completed the task"
`, nil
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 1, result.Iterations)
	require.NotNil(t, result.FinalStatus)
	require.Equal(t, "Phase 1", result.FinalStatus.PhaseCompleted)
}

func TestRunWithMockInvokerBlocked(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Stuck on something"
  error: "Cannot proceed"
`, nil
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonBlocked, result.ExitReason)
}

func TestRunWithMockInvokerNoStatus(t *testing.T) {
	mock := source.NewMockSource()
	callCount := 0
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		callCount++
		if callCount >= 4 {
			return &source.WorkItem{
				ID:    "test-123",
				Title: "Test Ticket",
				Phases: []source.Phase{
					{Name: "Phase 1", Completed: true},
				},
			}, nil
		}
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	invokeCount := 0
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		invokeCount++
		return "Some output without status block", nil
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.GreaterOrEqual(t, invokeCount, 2)
}

func TestRunWithMockInvokerError(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("claude error")
	})

	result, err := l.Run("test-123")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunMaxIterations(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Working on it"
`, nil
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonMaxIterations, result.ExitReason)
	require.Equal(t, 4, result.Iterations)
}

func TestRunStagnation(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "Thinking..."
`, nil
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason)
}

func TestRunFilesChanged(t *testing.T) {
	mock := source.NewMockSource()
	invocation := 0
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
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
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Len(t, result.TotalFilesChanged, 3)
}

func TestRunGetTicketErrorDuringLoop(t *testing.T) {
	mock := source.NewMockSource()
	callCount := 0
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		callCount++
		if callCount > 1 {
			return nil, fmt.Errorf("ticket fetch error")
		}
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
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

func TestSetClaudeInvoker(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	require.Nil(t, l.claudeInvoker)

	invoker := func(_ context.Context, _ string) (string, error) {
		return "test", nil
	}
	l.SetClaudeInvoker(invoker)

	require.NotNil(t, l.claudeInvoker)
}

func TestRunParseError(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  this is invalid yaml: [
`, nil
	})

	result, err := l.Run("test-123")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunContextCancellation(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	invocations := 0
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
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
	})

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonUserInterrupt, result.ExitReason)
}

// Tests for RunReviewOnly

func TestRunReviewOnlyPassesWithNoIssues(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

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
	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 2}
	l := New(config, "/tmp", nil, nil, false)

	// Mock review runner to always return issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return CONTINUE
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed one issue"
  commit_made: true
`, nil
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonMaxReviewRetries, result.ExitReason)
}

func TestRunReviewOnlyBlocked(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

	// Mock review runner to return issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return BLOCKED
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Cannot fix this issue"
  error: "Requires human intervention"
`, nil
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.False(t, result.Passed)
	require.Equal(t, safety.ExitReasonBlocked, result.ExitReason)
}

func TestRunReviewOnlyFixAndPass(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

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
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
  commit_made: true
`, nil
	})

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

	l.Stop()

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonUserInterrupt, result.ExitReason)
}

func TestRunReviewOnlyInvokerError(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

	// Mock review runner to return issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker to return error
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("claude error")
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunReviewOnlyTracksFilesFixed(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

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
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		claudeCall++
		files := fmt.Sprintf(`["file%d.go"]`, claudeCall)
		return fmt.Sprintf(`PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: %s
  summary: "Fixed issue %d"
  commit_made: true
`, files, claudeCall), nil
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Len(t, result.FilesFixed, 2) // Two different files fixed
}

func TestRunReviewOnlyAutoCommit(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

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
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
`, nil
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	// Should pass since review passes on second iteration
	require.NoError(t, err)
	require.True(t, result.Passed)
	require.Equal(t, 2, result.Iterations)
	require.Len(t, result.FilesFixed, 1)
	// Auto-commit would have been attempted but might fail in test env - that's OK
}

func TestBuildReviewFixPrompt(t *testing.T) {
	baseBranch := "main"
	filesChanged := []string{"main.go", "utils.go"}
	issuesMarkdown := "### quality\n- Error not handled at main.go:42"
	iteration := 2

	prompt := BuildReviewFixPrompt(baseBranch, filesChanged, issuesMarkdown, iteration)

	require.Contains(t, prompt, "Base branch: main")
	require.Contains(t, prompt, "Review iteration: 2")
	require.Contains(t, prompt, "main.go")
	require.Contains(t, prompt, "utils.go")
	require.Contains(t, prompt, issuesMarkdown)
	require.Contains(t, prompt, "PROGRAMMATOR_STATUS:")
	require.Contains(t, prompt, "commit_made: true")
}

func TestBuildReviewFixPromptFormatting(t *testing.T) {
	prompt := BuildReviewFixPrompt("develop", []string{"file.go"}, "some issues", 1)

	// Check structure
	require.Contains(t, prompt, "## Context")
	require.Contains(t, prompt, "## Files to review")
	require.Contains(t, prompt, "## Issues Found")
	require.Contains(t, prompt, "## Instructions")
	require.Contains(t, prompt, "## Session End Protocol")
}

// Helper functions for creating mock review runners

func createMockReviewRunner(t *testing.T, hasIssues bool, issueCount int) *review.Runner {
	t.Helper()

	cfg := review.Config{
		Enabled:       true,
		MaxIterations: 3,
		Passes: []review.Pass{
			{
				Name:     "test_pass",
				Parallel: true,
				Agents: []review.AgentConfig{
					{Name: "test_agent"},
				},
			},
		},
	}

	runner := review.NewRunner(cfg, nil)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
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
		Enabled:       true,
		MaxIterations: 3,
		Passes: []review.Pass{
			{
				Name:     "test_pass",
				Parallel: true,
				Agents: []review.AgentConfig{
					{Name: "test_agent"},
				},
			},
		},
	}

	runner := review.NewRunner(cfg, nil)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
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
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
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
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	// Eventually passes after review runner returns no issues
	require.True(t, result.Passed)
}

func TestRunReviewOnlyReviewError(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

	// Mock review runner that returns an error (via sequential execution to propagate error)
	cfg := review.Config{
		Enabled:       true,
		MaxIterations: 3,
		Passes: []review.Pass{
			{
				Name:     "test_pass",
				Parallel: false, // Sequential so error propagates
				Agents: []review.AgentConfig{
					{Name: "test_agent"},
				},
			},
		},
	}
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

	// When agent errors in sequential mode, the result includes the error but the pass itself doesn't fail.
	// The review passes with zero issues (since the error result has no issues).
	// This is current behavior - verifying it works as expected.
	require.NoError(t, err)
	require.True(t, result.Passed) // Passes because there are 0 issues
}

func TestRunReviewOnlyStagnation(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

	// Mock review runner that always returns issues
	mockRunner := createMockReviewRunner(t, true, 1)
	l.SetReviewRunner(mockRunner)

	// Mock Claude invoker that never changes any files (should trigger stagnation)
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "Thinking about how to fix this"
`, nil
	})

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

	stateCallback := func(state *safety.State, _ *source.WorkItem, _ []string) {
		callbackInvoked = true
		lastState = state
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, stateCallback, false)

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
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
  commit_made: true
`, nil
	})

	result, err := l.RunReviewOnly("main", []string{"file.go"})

	require.NoError(t, err)
	require.True(t, result.Passed)
	require.True(t, callbackInvoked, "state callback should have been invoked")
	require.NotNil(t, lastState)
}

func TestRunReviewOnlyDurationTracked(t *testing.T) {
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60, MaxReviewIterations: 10}
	l := New(config, "/tmp", nil, nil, false)

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
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file.go"]
  summary: "Fixed the issue"
  commit_made: true
`, nil
	})

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
		Enabled:       true,
		MaxIterations: 5,
	}
	l.SetReviewConfig(cfg)

	require.True(t, l.reviewConfig.Enabled)
	require.Equal(t, 5, l.reviewConfig.MaxIterations)
}

func TestSetSkipReview(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	require.False(t, l.skipReview)

	l.SetSkipReview(true)
	require.True(t, l.skipReview)
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

	// Mock Claude to complete first task, then second task
	invocation := 0
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
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
	})

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
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:         "phaseless-123",
			Title:      "Phaseless Ticket",
			Phases:     nil, // No phases - phaseless ticket
			RawContent: "# Phaseless Ticket\n\nJust do the task.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	// Mock Claude to report DONE on first invocation
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: DONE
  files_changed: ["main.go"]
  summary: "Completed the entire task"
`, nil
	})

	result, err := l.Run("phaseless-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 1, result.Iterations)
	require.NotNil(t, result.FinalStatus)
	require.Equal(t, parser.StatusDone, result.FinalStatus.Status)

	// Verify status was set to closed
	require.Len(t, mock.SetStatusCalls, 2) // in_progress + closed
	require.Equal(t, "closed", mock.SetStatusCalls[1].Status)
}

func TestRunPhaselessTicket_ContinuesUntilDone(t *testing.T) {
	// Test: A phaseless ticket continues looping until Claude signals DONE
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:         "phaseless-456",
			Title:      "Multi-iteration Phaseless Ticket",
			Phases:     []source.Phase{}, // Empty phases - also phaseless
			RawContent: "# Task\n\nComplex task requiring multiple steps.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	// Mock Claude to work for 3 iterations before reporting DONE
	invocation := 0
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
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
	})

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
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:         "phaseless-stag",
			Title:      "Phaseless Ticket That Stagnates",
			Phases:     nil,
			RawContent: "# Task\n\nTask that doesn't progress.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	// Mock Claude to never make progress (no files changed)
	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "Still thinking..."
`, nil
	})

	result, err := l.Run("phaseless-stag")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason)
}

func TestRunPhaselessTicket_BlockedHandled(t *testing.T) {
	// Test: BLOCKED status is handled correctly for phaseless tickets
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*source.WorkItem, error) {
		return &source.WorkItem{
			ID:         "phaseless-blocked",
			Title:      "Phaseless Ticket That Gets Blocked",
			Phases:     nil,
			RawContent: "# Task\n\nTask that gets blocked.\n",
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Cannot proceed"
  error: "Missing required credentials"
`, nil
	})

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
	require.Contains(t, settings, fmt.Sprintf("DCG_CONFIG=%s/.config/dcg/config.toml dcg", home))
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
	require.Contains(t, settings, fmt.Sprintf("DCG_CONFIG=%s/.config/dcg/config.toml dcg", home))
}

func TestSetGuardMode(t *testing.T) {
	l := New(safety.Config{}, "", nil, nil, false)

	require.False(t, l.guardMode)

	l.SetGuardMode(true)
	require.True(t, l.guardMode)
}

func TestWorkItemHelpers_Phaseless(t *testing.T) {
	// Test: WorkItem helper methods work correctly for phaseless items
	tests := []struct {
		name              string
		phases            []source.Phase
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
			phases:            []source.Phase{},
			hasPhases:         false,
			allPhasesComplete: false,
			currentPhaseIsNil: true,
		},
		{
			name: "has incomplete phases",
			phases: []source.Phase{
				{Name: "Phase 1", Completed: false},
			},
			hasPhases:         true,
			allPhasesComplete: false,
			currentPhaseIsNil: false,
		},
		{
			name: "all phases complete",
			phases: []source.Phase{
				{Name: "Phase 1", Completed: true},
			},
			hasPhases:         true,
			allPhasesComplete: true,
			currentPhaseIsNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item := &source.WorkItem{Phases: tc.phases}

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
