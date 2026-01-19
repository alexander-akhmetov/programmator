package loop

import (
	"strings"
	"testing"
	"time"

	"github.com/alexanderzobnin/programmator/internal/parser"
	"github.com/alexanderzobnin/programmator/internal/safety"
	"github.com/alexanderzobnin/programmator/internal/ticket"
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

	if !strings.Contains(logOutput, "[programmator]") {
		t.Error("log output should contain [programmator] prefix")
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
