package loop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/source"
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

	l := New(config, "/tmp", nil, false)

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

func TestLoopStop(t *testing.T) {
	config := safety.Config{}
	l := New(config, "", nil, false)

	l.Stop()

	if !l.stopRequested.Load() {
		t.Error("stopRequested should be true after Stop()")
	}
}

// NOTE: processTextOutput, processStreamingOutput, and timeoutBlockedStatus
// have been moved to internal/llm and are tested there.

func TestInvokeClaudePrintCapturesStderr(t *testing.T) {
	config := safety.Config{MaxIterations: 1, StagnationLimit: 1, Timeout: 10}
	l := New(config, "", nil, false)

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
	l := New(config, "", nil, false)

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
	l := New(config, "", stateCallback, false)

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

func TestLoopLogEvent(t *testing.T) {
	var received []event.Event
	config := safety.Config{}
	l := New(config, "", nil, false)
	l.SetEventCallback(func(e event.Event) {
		received = append(received, e)
	})

	l.log("test event message")

	require.Len(t, received, 1)
	require.Equal(t, event.KindProg, received[0].Kind)
	require.Equal(t, "test event message", received[0].Text)
}

func TestLogStartBanner(t *testing.T) {
	tests := []struct {
		name       string
		srcType    string
		itemID     string
		workItem   *domain.WorkItem
		wantSubs   []string // substrings that must appear in the banner
		rejectSubs []string // substrings that must NOT appear
	}{
		{
			name:    "plan with mixed phases",
			srcType: protocol.SourceTypePlan,
			itemID:  "./plan.md",
			workItem: &domain.WorkItem{
				Title: "Feature Name",
				Phases: []domain.Phase{
					{Name: "Investigation", Completed: true},
					{Name: "Implementation", Completed: false},
					{Name: "Testing", Completed: false},
				},
			},
			wantSubs: []string{
				"[programmator]",
				"Starting plan ./plan.md: Feature Name",
				"Tasks (3):",
				"✓ Investigation",
				"→ Implementation",
				"○ Testing",
			},
		},
		{
			name:    "ticket with phases",
			srcType: protocol.SourceTypeTicket,
			itemID:  "pro-123",
			workItem: &domain.WorkItem{
				Title: "Fix bug",
				Phases: []domain.Phase{
					{Name: "Investigate", Completed: false},
				},
			},
			wantSubs: []string{
				"Starting ticket pro-123: Fix bug",
				"Phases (1):",
				"→ Investigate",
			},
		},
		{
			name:    "no phases",
			srcType: protocol.SourceTypePlan,
			itemID:  "./simple.md",
			workItem: &domain.WorkItem{
				Title: "Simple Task",
			},
			wantSubs: []string{
				"Starting plan ./simple.md: Simple Task",
			},
			rejectSubs: []string{"Tasks", "Phases"},
		},
		{
			name:    "duplicate phase names marks only first as current",
			srcType: protocol.SourceTypePlan,
			itemID:  "./dup.md",
			workItem: &domain.WorkItem{
				Title: "Dup phases",
				Phases: []domain.Phase{
					{Name: "Run tests", Completed: true},
					{Name: "Run tests", Completed: false},
					{Name: "Run tests", Completed: false},
				},
			},
			wantSubs: []string{
				"✓ Run tests",
				"→ Run tests",
				"○ Run tests",
			},
		},
		{
			name:    "all phases completed",
			srcType: protocol.SourceTypeTicket,
			itemID:  "pro-done",
			workItem: &domain.WorkItem{
				Title: "Done ticket",
				Phases: []domain.Phase{
					{Name: "Step 1", Completed: true},
					{Name: "Step 2", Completed: true},
				},
			},
			wantSubs: []string{
				"Phases (2):",
				"✓ Step 1",
				"✓ Step 2",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var received []event.Event
			l := New(safety.Config{}, "", nil, false)
			l.SetEventCallback(func(e event.Event) {
				received = append(received, e)
			})

			l.logStartBanner(tc.srcType, tc.itemID, tc.workItem)

			require.Len(t, received, 1)
			require.Equal(t, event.KindIterationSeparator, received[0].Kind)
			for _, sub := range tc.wantSubs {
				require.Contains(t, received[0].Text, sub)
			}
			for _, sub := range tc.rejectSubs {
				require.NotContains(t, received[0].Text, sub)
			}
		})
	}
}

// TestLoopLogNoCallback verifies that log() does not panic when no callback is set.
func TestLoopLogNoCallback(t *testing.T) {
	_ = t // test passes if no panic occurs; named param allows future assertions
	config := safety.Config{}
	l := New(config, "", nil, false)

	l.log("test message")
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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", stateCallback, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	_, err := l.Run("test-123")
	require.NoError(t, err)
	require.True(t, callbackInvoked, "state callback should have been called")
}

func TestNewWithSourceNil(t *testing.T) {
	config := safety.Config{MaxIterations: 10}
	l := NewWithSource(config, "/tmp", nil, false, nil)

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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

	result, err := l.Run("test-123")

	require.Error(t, err)
	require.Equal(t, safety.ExitReasonError, result.ExitReason)
}

func TestSetInvoker(t *testing.T) {
	l := New(safety.Config{}, "", nil, false)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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

	runner := review.NewRunner(cfg)
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

	runner := review.NewRunner(cfg)
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

func TestSetReviewConfig(t *testing.T) {
	l := New(safety.Config{}, "", nil, false)

	cfg := review.Config{
		MaxIterations: 5,
	}
	l.SetReviewConfig(cfg)

	require.Equal(t, 5, l.reviewConfig.MaxIterations)
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
	l := NewWithSource(config, tmpDir, nil, false, planSource)
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

func TestRunIndexBasedPhaseCompletion(t *testing.T) {
	mock := source.NewMockSource()
	completedPhases := map[int]bool{}
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		phases := []domain.Phase{
			{Name: "Task 1", Completed: completedPhases[0]},
			{Name: "Task 2", Completed: completedPhases[1]},
		}
		return &domain.WorkItem{
			ID:     "test-idx",
			Title:  "Test Index Completion",
			Phases: phases,
		}, nil
	}
	mock.UpdatePhaseByIndexFunc = func(_ string, index int) error {
		completedPhases[index] = true
		return nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	invocation := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocation++
		if invocation == 1 {
			return `PROGRAMMATOR_STATUS:
  phase_completed: "Task 1"
  phase_completed_index: 1
  status: CONTINUE
  files_changed: ["file1.go"]
  summary: "Completed first task"
`, nil
		}
		return `PROGRAMMATOR_STATUS:
  phase_completed: "Task 2"
  phase_completed_index: 2
  status: DONE
  files_changed: ["file2.go"]
  summary: "Completed second task"
`, nil
	}})

	result, err := l.Run("test-idx")
	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 2, result.Iterations)

	// Should have called UpdatePhaseByIndex, not UpdatePhase
	require.Len(t, mock.UpdatePhaseByIndexCalls, 2)
	require.Equal(t, 0, mock.UpdatePhaseByIndexCalls[0].Index) // 1-based → 0-based
	require.Equal(t, 1, mock.UpdatePhaseByIndexCalls[1].Index)
	require.Empty(t, mock.UpdatePhaseCalls)
}

func TestRunIndexBasedFallsBackToNameBased(t *testing.T) {
	mock := source.NewMockSource()
	completedPhases := map[string]bool{}
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		phases := []domain.Phase{
			{Name: "Phase 1", Completed: completedPhases["Phase 1"]},
		}
		return &domain.WorkItem{
			ID:     "test-fallback",
			Title:  "Test Fallback",
			Phases: phases,
		}, nil
	}
	mock.UpdatePhaseFunc = func(_ string, name string) error {
		completedPhases[name] = true
		return nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		// No phase_completed_index — only name-based
		return `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: DONE
  files_changed: ["main.go"]
  summary: "Done"
`, nil
	}})

	result, err := l.Run("test-fallback")
	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Should have called UpdatePhase (name-based), not UpdatePhaseByIndex
	require.Len(t, mock.UpdatePhaseCalls, 1)
	require.Equal(t, "Phase 1", mock.UpdatePhaseCalls[0].PhaseName)
	require.Empty(t, mock.UpdatePhaseByIndexCalls)
}

func TestRunIndexBasedWithPlanSource(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := tmpDir + "/test-plan.md"
	content := `# Plan: Index Test

## Tasks
- [ ] Task 1: First task
- [ ] Task 2: Second task
`
	err := os.WriteFile(planPath, []byte(content), 0644)
	require.NoError(t, err)

	planSource := source.NewPlanSource(planPath)
	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, tmpDir, nil, false, planSource)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	invocation := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocation++
		if invocation == 1 {
			return `PROGRAMMATOR_STATUS:
  phase_completed_index: 1
  status: CONTINUE
  files_changed: ["file1.go"]
  summary: "Completed first task by index"
`, nil
		}
		return `PROGRAMMATOR_STATUS:
  phase_completed_index: 2
  status: DONE
  files_changed: ["file2.go"]
  summary: "Completed second task by index"
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

func TestRecordPhaseProgress(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name             string
		status           *parser.ParsedStatus
		wantByIndex      bool // expect UpdatePhaseByIndex
		wantByName       bool // expect UpdatePhase
		wantIndex        int  // expected 0-based index
		wantPhaseName    string
		wantNoteContains string
	}{
		{
			name: "index and name both set - uses index",
			status: &parser.ParsedStatus{
				PhaseCompletedIndex: intPtr(3),
				PhaseCompleted:      "Run tests",
				Summary:             "done",
			},
			wantByIndex:      true,
			wantIndex:        2, // 3 (1-based) → 2 (0-based)
			wantNoteContains: "Completed Run tests",
		},
		{
			name: "index only - uses task #N label",
			status: &parser.ParsedStatus{
				PhaseCompletedIndex: intPtr(1),
				Summary:             "done",
			},
			wantByIndex:      true,
			wantIndex:        0,
			wantNoteContains: "Completed task #1",
		},
		{
			name: "name only - falls back to name-based",
			status: &parser.ParsedStatus{
				PhaseCompleted: "Build project",
				Summary:        "done",
			},
			wantByName:       true,
			wantPhaseName:    "Build project",
			wantNoteContains: "Completed Build project",
		},
		{
			name: "neither index nor name - records summary",
			status: &parser.ParsedStatus{
				Summary: "Still working on it",
			},
			wantNoteContains: "Still working on it",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := source.NewMockSource()
			l := New(safety.Config{}, "", nil, false)

			rc := &runContext{
				workItemID: "test-1",
				source:     mock,
				state:      safety.NewState(),
			}
			rc.state.Iteration = 1

			l.recordPhaseProgress(rc, tc.status)

			if tc.wantByIndex {
				require.Len(t, mock.UpdatePhaseByIndexCalls, 1)
				require.Equal(t, tc.wantIndex, mock.UpdatePhaseByIndexCalls[0].Index)
				require.Empty(t, mock.UpdatePhaseCalls)
			} else if tc.wantByName {
				require.Len(t, mock.UpdatePhaseCalls, 1)
				require.Equal(t, tc.wantPhaseName, mock.UpdatePhaseCalls[0].PhaseName)
				require.Empty(t, mock.UpdatePhaseByIndexCalls)
			} else {
				require.Empty(t, mock.UpdatePhaseByIndexCalls)
				require.Empty(t, mock.UpdatePhaseCalls)
			}

			require.NotEmpty(t, mock.AddNoteCalls)
			require.Contains(t, mock.AddNoteCalls[0].Note, tc.wantNoteContains)
		})
	}
}

func TestRecordPhaseProgress_IndexErrorLogsWarning(t *testing.T) {
	mock := source.NewMockSource()
	mock.UpdatePhaseByIndexFunc = func(_ string, _ int) error {
		return fmt.Errorf("index out of range")
	}

	var logs []string
	l := New(safety.Config{}, "", nil, false)
	l.SetEventCallback(func(e event.Event) {
		logs = append(logs, e.Text)
	})

	rc := &runContext{
		workItemID: "test-1",
		source:     mock,
		state:      safety.NewState(),
	}
	rc.state.Iteration = 1

	idx := 5
	l.recordPhaseProgress(rc, &parser.ParsedStatus{
		PhaseCompletedIndex: &idx,
		Summary:             "done",
	})

	// Should have logged a warning about the error
	var foundWarning bool
	for _, log := range logs {
		if strings.Contains(log, "Warning: failed to update phase by index") {
			foundWarning = true
			break
		}
	}
	require.True(t, foundWarning, "expected warning log about index error, got: %v", logs)

	// Note should still be added despite the error
	require.NotEmpty(t, mock.AddNoteCalls)
	require.Contains(t, mock.AddNoteCalls[0].Note, "Completed task #5")
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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)
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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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
	l := NewWithSource(config, "", nil, false, mock)

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

	runner := review.NewRunner(cfg)
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

// TestReviewAgentErrorsHaveRetryLimit verifies that persistent agent errors
// don't cause an infinite loop. The bug: when review agents error, the loop
// decrements ReviewIterations and returns loopRetryReview, but the main loop
// skips incrementing the iteration counter, so safety checks never fire.
func TestReviewAgentErrorsHaveRetryLimit(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-infinite-loop",
			Title: "Test Infinite Loop Bug",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true}, // All phases complete triggers review
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 10}
	l := NewWithSource(config, "", nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 5,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Review runner that ALWAYS returns agent errors
	agentErrorCount := 0
	runner := createMockReviewRunnerWithErrors(t, func() (agentError bool, hasIssues bool) {
		agentErrorCount++
		return true, false // Always return agent error
	})
	l.SetReviewRunner(runner)

	// Claude should never be invoked since we're stuck retrying review
	claudeInvoked := false
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		claudeInvoked = true
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "fix"
`, nil
	}})

	result, err := l.Run("test-infinite-loop")

	require.NoError(t, err)
	// Should exit due to stagnation (agent errors count as no progress)
	require.Equal(t, safety.ExitReasonStagnation, result.ExitReason,
		"should exit due to stagnation when agents persistently fail")
	// Should have a bounded number of retries based on stagnation limit (10 in this test)
	require.LessOrEqual(t, agentErrorCount, 11,
		"agent errors should be bounded by stagnation limit")
	require.False(t, claudeInvoked,
		"Claude should not be invoked when review agents keep erroring")
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

// Regression tests: lock down core run/review behaviors before pipeline migration.
// These tests verify behaviors that must survive the refactoring from loop to pipeline.

// TestRunPhaseCompletionTriggersReviewRunner verifies that after all phases complete,
// the review runner is invoked. This is a key behavior: work → review → complete.
func TestRunPhaseCompletionTriggersReviewRunner(t *testing.T) {
	mock := source.NewMockSource()
	phaseCompleted := false
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-review-trigger",
			Title: "Test Review Trigger",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: phaseCompleted},
			},
		}, nil
	}
	mock.UpdatePhaseFunc = func(_, _ string) error {
		phaseCompleted = true
		return nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60, MaxReviewIterations: 10}
	l := NewWithSource(config, "", nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Track whether review runner was actually invoked
	reviewRunnerInvoked := false
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewRunnerInvoked = true
		return false, 0 // pass
	})
	l.SetReviewRunner(mockRunner)

	// Mock Claude to complete the single phase
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: CONTINUE
  files_changed: ["main.go"]
  summary: "Completed the phase"
`, nil
	}})

	result, err := l.Run("test-review-trigger")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.True(t, reviewRunnerInvoked, "review runner must be invoked after all phases complete")
}

// TestRunResultDurationTracked verifies that Result.Duration is populated after Run.
func TestRunResultDurationTracked(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-duration",
			Title: "Test Duration",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 10, StagnationLimit: 3, Timeout: 60}
	l := NewWithSource(config, "", nil, false, mock)
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	result, err := l.Run("test-duration")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Greater(t, result.Duration, time.Duration(0), "Result.Duration should be > 0")
}

// TestRunRecentSummariesPopulated verifies that Result.RecentSummaries accumulates
// across multiple iterations. This data is used for stagnation diagnosis.
func TestRunRecentSummariesPopulated(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-summaries",
			Title: "Test Summaries",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 3, StagnationLimit: 10, Timeout: 60}
	l := NewWithSource(config, "", nil, false, mock)

	invocation := 0
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		invocation++
		return fmt.Sprintf(`PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["file%d.go"]
  summary: "Iteration %d work"
`, invocation, invocation), nil
	}})

	result, err := l.Run("test-summaries")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonMaxIterations, result.ExitReason)
	require.NotEmpty(t, result.RecentSummaries, "RecentSummaries should be populated")
	require.GreaterOrEqual(t, len(result.RecentSummaries), 2, "should have summaries from multiple iterations")
}

// TestRunEventEmissionDuringFullRun verifies that events are emitted during Run
// when an event callback is set.
func TestRunEventEmissionDuringFullRun(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-events",
			Title: "Test Events",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}

	var receivedEvents []event.Event
	config := safety.Config{MaxIterations: 10, StagnationLimit: 5, Timeout: 60}
	l := NewWithSource(config, "", nil, false, mock)
	l.SetEventCallback(func(e event.Event) {
		receivedEvents = append(receivedEvents, e)
	})
	l.SetReviewConfig(singleAgentReviewConfig())
	l.SetReviewRunner(createMockReviewRunner(t, false, 0))

	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, _ string) (string, error) {
		return `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: DONE
  files_changed: ["main.go"]
  summary: "Done"
`, nil
	}})

	result, err := l.Run("test-events")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.NotEmpty(t, receivedEvents, "events should be emitted during Run")

	// Verify we got at least a Prog event (iteration separator or status log)
	var hasProgEvent bool
	for _, e := range receivedEvents {
		if e.Kind == event.KindProg {
			hasProgEvent = true
			break
		}
	}
	require.True(t, hasProgEvent, "should emit at least one Prog event during Run")
}

// TestRunReviewFixCycleInMainLoop verifies the complete review-fix cycle within Run:
// phases complete → review finds issues → Claude fixes → review passes → complete.
func TestRunReviewFixCycleInMainLoop(t *testing.T) {
	mock := source.NewMockSource()
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-review-fix",
			Title: "Test Review Fix Cycle",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
			},
		}, nil
	}

	config := safety.Config{MaxIterations: 50, StagnationLimit: 10, Timeout: 60, MaxReviewIterations: 50}
	l := NewWithSource(config, "", nil, false, mock)

	l.SetReviewConfig(review.Config{
		MaxIterations: 10,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Review: first call finds issues, second call passes
	reviewCall := 0
	mockRunner := createMockReviewRunnerFunc(t, func() (bool, int) {
		reviewCall++
		if reviewCall == 1 {
			return true, 2 // has 2 issues
		}
		return false, 0 // passes
	})
	l.SetReviewRunner(mockRunner)

	// Claude fixes the issues
	var fixPromptReceived string
	l.SetInvoker(&fakeInvoker{fn: func(_ context.Context, p string) (string, error) {
		fixPromptReceived = p
		return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: ["fix.go"]
  summary: "Fixed review issues"
`, nil
	}})

	result, err := l.Run("test-review-fix")

	require.NoError(t, err)
	require.Equal(t, safety.ExitReasonComplete, result.ExitReason)
	require.Equal(t, 2, reviewCall, "review runner should be called twice (fail then pass)")
	require.NotEmpty(t, fixPromptReceived, "Claude should receive a fix prompt")
	require.Contains(t, result.TotalFilesChanged, "fix.go", "fixed files should be tracked")
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
	l := NewWithSource(cfg, "", nil, false, mock)
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

func TestHandleToolResult(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		result        string
		wantEvent     bool
		wantContains  string
		wantEventKind event.Kind
	}{
		{
			name:      "no callbacks set",
			toolName:  "Read",
			result:    "line1\nline2\n",
			wantEvent: false,
		},
		{
			name:      "empty tool name",
			toolName:  "",
			result:    "some result",
			wantEvent: false,
		},
		{
			name:          "Read tool with lines",
			toolName:      "Read",
			result:        "line1\nline2\nline3\n",
			wantEvent:     true,
			wantContains:  "Read 3 lines",
			wantEventKind: event.KindToolResult,
		},
		{
			name:          "Write tool",
			toolName:      "Write",
			result:        "success",
			wantEvent:     true,
			wantContains:  "File written",
			wantEventKind: event.KindToolResult,
		},
		{
			name:          "Edit tool",
			toolName:      "Edit",
			result:        "success",
			wantEvent:     true,
			wantContains:  "File updated",
			wantEventKind: event.KindToolResult,
		},
		{
			name:          "Bash with output",
			toolName:      "Bash",
			result:        "hello world",
			wantEvent:     true,
			wantContains:  "hello world",
			wantEventKind: event.KindToolResult,
		},
		{
			name:      "empty result",
			toolName:  "Read",
			result:    "",
			wantEvent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var eventReceived event.Event
			var eventCalled bool

			l := New(safety.Config{}, "/tmp", nil, false)

			if tc.wantEvent {
				l.onEvent = func(e event.Event) {
					eventCalled = true
					eventReceived = e
				}
			}

			l.handleToolResult(tc.toolName, tc.result)

			if tc.wantEvent {
				require.True(t, eventCalled, "event callback should be called")
				require.Equal(t, tc.wantEventKind, eventReceived.Kind)
				require.Contains(t, eventReceived.Text, tc.wantContains)
			} else {
				require.False(t, eventCalled, "event callback should not be called")
			}
		})
	}
}

func TestOutputToolUse(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		input         any
		wantContains  string
		wantEventKind event.Kind
	}{
		{
			name:          "Read with file path",
			toolName:      "Read",
			input:         map[string]any{"file_path": "/foo/bar.go"},
			wantContains:  "Read /foo/bar.go",
			wantEventKind: event.KindToolUse,
		},
		{
			name:          "Bash with command",
			toolName:      "Bash",
			input:         map[string]any{"command": "git status"},
			wantContains:  "Bash git status",
			wantEventKind: event.KindToolUse,
		},
		{
			name:          "Glob with pattern",
			toolName:      "Glob",
			input:         map[string]any{"pattern": "**/*.go"},
			wantContains:  "Glob **/*.go",
			wantEventKind: event.KindToolUse,
		},
		{
			name:          "tool without input",
			toolName:      "SomeTool",
			input:         nil,
			wantContains:  "SomeTool",
			wantEventKind: event.KindToolUse,
		},
		{
			name:          "tool with non-map input",
			toolName:      "Custom",
			input:         "string input",
			wantContains:  "Custom",
			wantEventKind: event.KindToolUse,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var eventReceived event.Event

			l := New(safety.Config{}, "/tmp", nil, false)
			l.onEvent = func(e event.Event) {
				eventReceived = e
			}

			l.outputToolUse(tc.toolName, tc.input)

			require.Equal(t, tc.wantEventKind, eventReceived.Kind)
			require.Contains(t, eventReceived.Text, tc.wantContains)
		})
	}
}

func TestOutputToolUseNoCallback(_ *testing.T) {
	l := New(safety.Config{}, "/tmp", nil, false)
	// No callbacks set - should not panic
	l.outputToolUse("Read", map[string]any{"file_path": "/foo.go"})
}

func TestOutputEditDiff(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		wantHunk    string
		wantDiffAdd bool
		wantDiffDel bool
	}{
		{
			name: "add lines",
			input: map[string]any{
				"old_string": "line1\n",
				"new_string": "line1\nline2\nline3\n",
			},
			wantHunk:    "Added 2 lines",
			wantDiffAdd: true,
			wantDiffDel: false,
		},
		{
			name: "remove lines",
			input: map[string]any{
				"old_string": "line1\nline2\nline3\n",
				"new_string": "line1\n",
			},
			wantHunk:    "removed 2 lines",
			wantDiffDel: true,
			wantDiffAdd: false,
		},
		{
			name: "modify same line count",
			input: map[string]any{
				"old_string": "old content\n",
				"new_string": "new content\n",
			},
			wantHunk:    "Modified 1 line",
			wantDiffAdd: true,
			wantDiffDel: true,
		},
		{
			name: "missing old_string",
			input: map[string]any{
				"new_string": "content\n",
			},
			wantHunk: "", // should not output anything
		},
		{
			name: "missing new_string",
			input: map[string]any{
				"old_string": "content\n",
			},
			wantHunk: "", // should not output anything
		},
		{
			name: "add single line",
			input: map[string]any{
				"old_string": "",
				"new_string": "single line\n",
			},
			wantHunk:    "Added 1 line",
			wantDiffAdd: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var events []event.Event

			l := New(safety.Config{}, "/tmp", nil, false)
			l.onEvent = func(e event.Event) {
				events = append(events, e)
			}

			l.outputEditDiff(tc.input)

			if tc.wantHunk == "" {
				require.Empty(t, events, "should not emit events for invalid input")
				return
			}

			// Find the hunk event
			var hunkFound bool
			var diffAddFound, diffDelFound bool
			for _, e := range events {
				switch e.Kind {
				case event.KindDiffHunk:
					hunkFound = true
					require.Contains(t, e.Text, tc.wantHunk)
				case event.KindDiffAdd:
					diffAddFound = true
				case event.KindDiffDel:
					diffDelFound = true
				case event.KindProg, event.KindToolUse, event.KindToolResult,
					event.KindReview, event.KindDiffCtx, event.KindMarkdown,
					event.KindStreamingText, event.KindIterationSeparator:
					// Not relevant for this test
				}
			}

			require.True(t, hunkFound, "should emit DiffHunk event")
			require.Equal(t, tc.wantDiffAdd, diffAddFound, "DiffAdd event presence mismatch")
			require.Equal(t, tc.wantDiffDel, diffDelFound, "DiffDel event presence mismatch")
		})
	}
}

func TestOutputToolUseTriggersEditDiff(t *testing.T) {
	var events []event.Event

	l := New(safety.Config{}, "/tmp", nil, false)
	l.onEvent = func(e event.Event) {
		events = append(events, e)
	}

	// outputToolUse for "Edit" should also call outputEditDiff
	l.outputToolUse("Edit", map[string]any{
		"file_path":  "/test.go",
		"old_string": "old\n",
		"new_string": "new\n",
	})

	// Should have ToolUse event plus diff events
	var toolUseFound, diffHunkFound bool
	for _, e := range events {
		if e.Kind == event.KindToolUse {
			toolUseFound = true
		}
		if e.Kind == event.KindDiffHunk {
			diffHunkFound = true
		}
	}

	require.True(t, toolUseFound, "should emit ToolUse event")
	require.True(t, diffHunkFound, "should emit DiffHunk event for Edit tool")
}
