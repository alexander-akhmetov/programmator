package loop

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/ticket"
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
	var receivedTicket *ticket.Ticket

	stateCallback := func(state *safety.State, tk *ticket.Ticket, _ []string) {
		callbackCalled = true
		receivedState = state
		receivedTicket = tk
	}

	config := safety.Config{}
	l := New(config, "", nil, stateCallback, false)

	if l.onStateChange == nil {
		t.Fatal("onStateChange callback should be set")
	}

	testState := safety.NewState()
	testTicket := &ticket.Ticket{ID: "test-123"}

	l.onStateChange(testState, testTicket, nil)

	if !callbackCalled {
		t.Error("state callback should have been called")
	}
	if receivedState != testState {
		t.Error("received state doesn't match")
	}
	if receivedTicket != testTicket {
		t.Error("received ticket doesn't match")
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

	if !strings.Contains(logOutput, "programmator:") {
		t.Error("log output should contain programmator: prefix")
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
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 0, result.Iterations)
	require.Len(t, mock.SetStatusCalls, 2)
}

func TestRunGetTicketError(t *testing.T) {
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return nil, fmt.Errorf("ticket not found")
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

	result, err := l.Run("nonexistent")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunStopRequested(t *testing.T) {
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

	l.Stop()

	result, err := l.Run("test-123")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonUserInterrupt, result.ExitReason)
}

func TestRunStateCallbackCalled(t *testing.T) {
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	var callbackInvoked bool
	stateCallback := func(_ *safety.State, tkt *ticket.Ticket, _ []string) {
		callbackInvoked = true
		require.Equal(t, "test-123", tkt.ID)
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, stateCallback, false, mock)

	_, err := l.Run("test-123")
	require.NoError(t, err)
	require.True(t, callbackInvoked, "state callback should have been called")
}

func TestNewWithClientNil(t *testing.T) {
	config := safety.Config{MaxIterations: 10}
	l := NewWithClient(config, "/tmp", nil, nil, false, nil)

	require.NotNil(t, l)
	require.Nil(t, l.client)
}

func TestRunWithMockInvokerDone(t *testing.T) {
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	callCount := 0
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		callCount++
		if callCount >= 4 {
			return &ticket.Ticket{
				ID:    "test-123",
				Title: "Test Ticket",
				Phases: []ticket.Phase{
					{Name: "Phase 1", Completed: true},
				},
			}, nil
		}
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

	l.SetClaudeInvoker(func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("claude error")
	})

	result, err := l.Run("test-123")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestRunMaxIterations(t *testing.T) {
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 2, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	invocation := 0
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	callCount := 0
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		callCount++
		if callCount > 1 {
			return nil, fmt.Errorf("ticket fetch error")
		}
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
	mock := ticket.NewMockClient()
	mock.GetFunc = func(_ string) (*ticket.Ticket, error) {
		return &ticket.Ticket{
			ID:    "test-123",
			Title: "Test Ticket",
			Phases: []ticket.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithClient(config, "", nil, nil, false, mock)

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
