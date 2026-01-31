package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/parser"
	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/safety"
)

func newTestEngine() *Engine {
	return &Engine{
		SafetyConfig: safety.Config{
			MaxIterations:   50,
			StagnationLimit: 3,
			Timeout:         60,
		},
		ReviewPhaseCount: 1,
		PhaseMaxIterFunc: func(_ int) int { return 3 },
	}
}

func TestDecideNext(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(e *Engine)
		stopped        bool
		ctxDone        bool
		workItem       *domain.WorkItem
		taskCompleted  bool
		wantKind       ActionKind
		wantExitReason safety.ExitReason
		wantReviewFix  bool
		wantIterations int // only checked if > 0
	}{
		{
			name:           "stop requested",
			stopped:        true,
			workItem:       &domain.WorkItem{Phases: []domain.Phase{{Name: "P1"}}},
			wantKind:       ActionExit,
			wantExitReason: safety.ExitReasonUserInterrupt,
			wantIterations: 5,
		},
		{
			name:           "context canceled",
			ctxDone:        true,
			workItem:       &domain.WorkItem{Phases: []domain.Phase{{Name: "P1"}}},
			wantKind:       ActionExit,
			wantExitReason: safety.ExitReasonUserInterrupt,
		},
		{
			name:     "incomplete phases invoke LLM",
			workItem: &domain.WorkItem{Phases: []domain.Phase{{Name: "P1"}}},
			wantKind: ActionInvokeLLM,
		},
		{
			name:     "all phases complete, review not passed",
			workItem: &domain.WorkItem{Phases: []domain.Phase{{Name: "P1", Completed: true}}},
			wantKind: ActionRunReview,
		},
		{
			name:     "all phases complete, review passed",
			setup:    func(e *Engine) { e.ReviewPassed = true },
			workItem: &domain.WorkItem{Phases: []domain.Phase{{Name: "P1", Completed: true}}},
			wantKind: ActionComplete,
		},
		{
			name:          "task completed, review not passed",
			workItem:      &domain.WorkItem{Phases: []domain.Phase{{Name: "P1"}}},
			taskCompleted: true,
			wantKind:      ActionRunReview,
		},
		{
			name:          "pending review fix",
			setup:         func(e *Engine) { e.PendingReviewFix = true },
			workItem:      &domain.WorkItem{Phases: []domain.Phase{{Name: "P1", Completed: true}}},
			wantKind:      ActionInvokeLLM,
			wantReviewFix: true,
		},
		{
			name:     "review only mode",
			setup:    func(e *Engine) { e.ReviewOnly = true },
			workItem: &domain.WorkItem{Phases: []domain.Phase{{Name: "P1"}}},
			wantKind: ActionRunReview,
		},
		{
			name:     "nil workItem invokes LLM",
			workItem: nil,
			wantKind: ActionInvokeLLM,
		},
		{
			name:     "phaseless, not completed",
			workItem: &domain.WorkItem{Phases: nil},
			wantKind: ActionInvokeLLM,
		},
		{
			name:          "phaseless, completed",
			workItem:      &domain.WorkItem{Phases: nil},
			taskCompleted: true,
			wantKind:      ActionRunReview,
		},
		{
			name:           "stop takes precedence over completion",
			setup:          func(e *Engine) { e.ReviewPassed = true },
			stopped:        true,
			workItem:       &domain.WorkItem{Phases: []domain.Phase{{Name: "P1", Completed: true}}},
			taskCompleted:  true,
			wantKind:       ActionExit,
			wantExitReason: safety.ExitReasonUserInterrupt,
		},
		{
			name:           "context canceled takes precedence over completion",
			setup:          func(e *Engine) { e.ReviewPassed = true },
			ctxDone:        true,
			workItem:       &domain.WorkItem{Phases: []domain.Phase{{Name: "P1", Completed: true}}},
			taskCompleted:  true,
			wantKind:       ActionExit,
			wantExitReason: safety.ExitReasonUserInterrupt,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			if tc.setup != nil {
				tc.setup(e)
			}
			state := safety.NewState()
			if tc.wantIterations > 0 {
				state.Iteration = tc.wantIterations
			}

			action := e.DecideNext(tc.stopped, tc.ctxDone, tc.workItem, tc.taskCompleted, state)

			require.Equal(t, tc.wantKind, action.Kind)
			if tc.wantExitReason != "" {
				require.Equal(t, tc.wantExitReason, action.ExitReason)
			}
			if tc.wantReviewFix {
				require.True(t, action.IsReviewFix)
			}
			if tc.wantIterations > 0 {
				require.Equal(t, tc.wantIterations, action.Iterations)
			}
		})
	}
}

func TestCheckSafety(t *testing.T) {
	tests := []struct {
		name           string
		iteration      int
		noChanges      int
		wantExit       bool
		wantExitReason safety.ExitReason
	}{
		{
			name:      "OK",
			iteration: 1,
			wantExit:  false,
		},
		{
			name:      "exactly at max iterations",
			iteration: 50,
			wantExit:  false,
		},
		{
			name:           "max iterations exceeded",
			iteration:      51,
			wantExit:       true,
			wantExitReason: safety.ExitReasonMaxIterations,
		},
		{
			name:           "stagnation",
			noChanges:      3,
			wantExit:       true,
			wantExitReason: safety.ExitReasonStagnation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			state := safety.NewState()
			state.Iteration = tc.iteration
			state.ConsecutiveNoChanges = tc.noChanges

			result := e.CheckSafety(state)

			require.Equal(t, tc.wantExit, result.ShouldExit)
			if tc.wantExitReason != "" {
				require.Equal(t, tc.wantExitReason, result.ExitReason)
			}
		})
	}
}

func TestProcessStatus(t *testing.T) {
	tests := []struct {
		name                   string
		status                 *parser.ParsedStatus
		pendingReviewFix       bool
		wantTaskCompleted      bool
		wantBlocked            bool
		wantShouldExit         bool
		wantExitReason         safety.ExitReason
		wantResetPendingReview bool
		wantPhaseCompleted     string
		wantBlockedError       string
		wantFilesChanged       []string
	}{
		{
			name: "CONTINUE with files changed",
			status: &parser.ParsedStatus{
				PhaseCompleted: "Phase 1",
				Status:         protocol.StatusContinue,
				FilesChanged:   []string{"main.go"},
				Summary:        "Did work",
			},
			wantPhaseCompleted: "Phase 1",
			wantFilesChanged:   []string{"main.go"},
		},
		{
			name: "DONE",
			status: &parser.ParsedStatus{
				Status:  protocol.StatusDone,
				Summary: "All done",
			},
			wantTaskCompleted: true,
		},
		{
			name: "BLOCKED",
			status: &parser.ParsedStatus{
				Status:  protocol.StatusBlocked,
				Summary: "Stuck",
				Error:   "Missing dependency",
			},
			wantBlocked:      true,
			wantShouldExit:   true,
			wantExitReason:   safety.ExitReasonBlocked,
			wantBlockedError: "Missing dependency",
		},
		{
			name: "resets pending review fix",
			status: &parser.ParsedStatus{
				Status:  protocol.StatusContinue,
				Summary: "Fixed issues",
			},
			pendingReviewFix:       true,
			wantResetPendingReview: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			result := e.ProcessStatus(ProcessStatusInput{
				Status:           tc.status,
				Iteration:        1,
				PendingReviewFix: tc.pendingReviewFix,
			})

			require.Equal(t, tc.wantTaskCompleted, result.TaskCompleted)
			require.Equal(t, tc.wantBlocked, result.Blocked)
			require.Equal(t, tc.wantShouldExit, result.ShouldExit)
			if tc.wantExitReason != "" {
				require.Equal(t, tc.wantExitReason, result.ExitReason)
			}
			if tc.wantPhaseCompleted != "" {
				require.Equal(t, tc.wantPhaseCompleted, result.PhaseCompleted)
			}
			if tc.wantBlockedError != "" {
				require.Equal(t, tc.wantBlockedError, result.BlockedError)
			}
			require.Equal(t, tc.wantResetPendingReview, result.ResetPendingReviewFix)
			if tc.status != nil {
				require.Equal(t, tc.status.FilesChanged, result.FilesChanged)
			}
			if tc.wantFilesChanged != nil {
				require.Equal(t, tc.wantFilesChanged, result.FilesChanged)
			}
		})
	}
}

func TestProcessStatusNilStatus(t *testing.T) {
	e := newTestEngine()
	result := e.ProcessStatus(ProcessStatusInput{
		Status:    nil,
		Iteration: 1,
	})
	require.False(t, result.TaskCompleted)
	require.False(t, result.Blocked)
	require.False(t, result.ShouldExit)
	require.Empty(t, result.PhaseCompleted)
	require.Empty(t, result.FilesChanged)
}

func TestDecideReview(t *testing.T) {
	tests := []struct {
		name              string
		setup             func(e *Engine)
		passed            bool
		wantExitError     bool
		wantAllPhasesDone bool
		wantPhasePassed   bool
		wantAdvancePhase  bool
		wantNeedsFix      bool
		wantExceededLimit bool
		wantReviewPassed  bool
	}{
		{
			name:          "no phases configured",
			setup:         func(e *Engine) { e.ReviewPhaseCount = 0 },
			wantExitError: true,
		},
		{
			name: "all phases already done",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 2
				e.CurrentPhaseIdx = 2
			},
			wantAllPhasesDone: true,
		},
		{
			name: "passed, more phases remain",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 3
				e.CurrentPhaseIdx = 0
			},
			passed:           true,
			wantPhasePassed:  true,
			wantAdvancePhase: true,
		},
		{
			name: "passed, last phase",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 1
				e.CurrentPhaseIdx = 0
			},
			passed:            true,
			wantPhasePassed:   true,
			wantAdvancePhase:  true,
			wantAllPhasesDone: true,
			wantReviewPassed:  true,
		},
		{
			name: "failed within limit",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 1
				e.CurrentPhaseIdx = 0
				e.CurrentPhaseIter = 0
				e.PhaseMaxIterFunc = func(_ int) int { return 3 }
			},
			wantNeedsFix: true,
		},
		{
			name: "failed at exact limit boundary",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 1
				e.CurrentPhaseIdx = 0
				e.CurrentPhaseIter = 2
				e.PhaseMaxIterFunc = func(_ int) int { return 3 }
			},
			wantNeedsFix: true,
		},
		{
			name: "exceeded limit, more phases",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 2
				e.CurrentPhaseIdx = 0
				e.CurrentPhaseIter = 3
				e.PhaseMaxIterFunc = func(_ int) int { return 3 }
			},
			wantExceededLimit: true,
			wantAdvancePhase:  true,
		},
		{
			name: "exceeded limit, last phase",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 1
				e.CurrentPhaseIdx = 0
				e.CurrentPhaseIter = 3
				e.PhaseMaxIterFunc = func(_ int) int { return 3 }
			},
			wantExceededLimit: true,
			wantAdvancePhase:  true,
			wantAllPhasesDone: true,
			wantReviewPassed:  true,
		},
		{
			name: "nil PhaseMaxIterFunc treats limit as zero",
			setup: func(e *Engine) {
				e.ReviewPhaseCount = 1
				e.CurrentPhaseIdx = 0
				e.CurrentPhaseIter = 0
				e.PhaseMaxIterFunc = nil
			},
			wantExceededLimit: true,
			wantAdvancePhase:  true,
			wantAllPhasesDone: true,
			wantReviewPassed:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			if tc.setup != nil {
				tc.setup(e)
			}

			decision := e.DecideReview(tc.passed)

			if tc.wantExitError {
				require.NotEmpty(t, decision.ExitError)
				return
			}
			require.Equal(t, tc.wantAllPhasesDone, decision.AllPhasesDone)
			require.Equal(t, tc.wantPhasePassed, decision.PhasePassed)
			require.Equal(t, tc.wantAdvancePhase, decision.AdvancePhase)
			require.Equal(t, tc.wantNeedsFix, decision.NeedsFix)
			require.Equal(t, tc.wantExceededLimit, decision.ExceededLimit)
			if tc.wantExceededLimit {
				require.False(t, e.PendingReviewFix, "PendingReviewFix should be reset after exceeding limit")
			}
			if tc.wantReviewPassed {
				require.True(t, e.ReviewPassed)
			}
		})
	}
}

func TestFormatIterationSummary(t *testing.T) {
	tests := []struct {
		name     string
		iter     int
		summary  string
		files    []string
		expected string
	}{
		{
			name:     "with files",
			iter:     2,
			summary:  "Did work",
			files:    []string{"a.go", "b.go"},
			expected: "[iter 2] Did work (files: a.go, b.go)",
		},
		{
			name:     "no files",
			iter:     5,
			summary:  "Thinking",
			files:    nil,
			expected: "[iter 5] Thinking (no files changed)",
		},
		{
			name:     "single file",
			iter:     1,
			summary:  "Added feature",
			files:    []string{"main.go"},
			expected: "[iter 1] Added feature (files: main.go)",
		},
		{
			name:     "empty summary",
			iter:     3,
			summary:  "",
			files:    []string{"a.go"},
			expected: "[iter 3]  (files: a.go)",
		},
		{
			name:     "empty files slice",
			iter:     1,
			summary:  "Thinking",
			files:    []string{},
			expected: "[iter 1] Thinking (no files changed)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := FormatIterationSummary(tc.iter, tc.summary, tc.files)
			require.Equal(t, tc.expected, s)
		})
	}
}

func TestResetReviewState(t *testing.T) {
	e := newTestEngine()
	e.CurrentPhaseIdx = 2
	e.CurrentPhaseIter = 5
	e.PendingReviewFix = true
	e.ReviewPassed = true

	e.ResetReviewState()

	require.Equal(t, 0, e.CurrentPhaseIdx)
	require.Equal(t, 0, e.CurrentPhaseIter)
	require.False(t, e.PendingReviewFix)
	require.False(t, e.ReviewPassed)
}
