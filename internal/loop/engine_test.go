package loop

import (
	"testing"

	"github.com/stretchr/testify/require"

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
		MaxReviewIter: 10,
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
		wantPassed        bool
		wantNeedsFix      bool
		wantExceededLimit bool
		wantReviewPassed  bool
	}{
		{
			name:             "passed",
			passed:           true,
			wantPassed:       true,
			wantReviewPassed: true,
		},
		{
			name:         "failed within limit",
			wantNeedsFix: true,
		},
		{
			name: "failed at limit boundary",
			setup: func(e *Engine) {
				e.ReviewIterations = 8 // will be incremented to 9, which is < 10
			},
			wantNeedsFix: true,
		},
		{
			name: "exceeded limit",
			setup: func(e *Engine) {
				e.ReviewIterations = 9 // will be incremented to 10, which >= 10
			},
			wantExceededLimit: true,
			wantReviewPassed:  true,
		},
		{
			name: "no limit set (MaxReviewIter=0)",
			setup: func(e *Engine) {
				e.MaxReviewIter = 0
				e.ReviewIterations = 100
			},
			wantNeedsFix: true, // no limit, always needs fix
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			if tc.setup != nil {
				tc.setup(e)
			}

			decision := e.DecideReview(tc.passed)

			require.Equal(t, tc.wantPassed, decision.Passed)
			require.Equal(t, tc.wantNeedsFix, decision.NeedsFix)
			require.Equal(t, tc.wantExceededLimit, decision.ExceededLimit)
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

func TestDecideReview_SingleIterationLimit(t *testing.T) {
	// Verify that there's no per-phase iteration budget — only a single
	// review.max_iterations controls the entire review loop.
	e := newTestEngine()
	e.MaxReviewIter = 3

	// Simulate 3 failed iterations
	for i := range 2 {
		decision := e.DecideReview(false)
		require.True(t, decision.NeedsFix, "iteration %d should need fix", i+1)
		require.False(t, decision.ExceededLimit)
		require.False(t, decision.Passed)
		// Reset PendingReviewFix as the loop would
		e.PendingReviewFix = false
	}

	// Third failure should hit the limit
	decision := e.DecideReview(false)
	require.True(t, decision.ExceededLimit, "should exceed limit at iteration 3")
	require.False(t, decision.NeedsFix)
	require.True(t, e.ReviewPassed, "ReviewPassed should be set when limit exceeded")
}

func TestDecideReview_PassStopsImmediately(t *testing.T) {
	e := newTestEngine()
	e.MaxReviewIter = 10

	// Even after several failures, passing stops immediately
	e.DecideReview(false) // iteration 1
	e.PendingReviewFix = false
	e.DecideReview(false) // iteration 2
	e.PendingReviewFix = false

	decision := e.DecideReview(true)
	require.True(t, decision.Passed)
	require.True(t, e.ReviewPassed)
	require.Equal(t, 2, e.ReviewIterations, "iterations should not increment on pass")
}

func TestResetReviewState(t *testing.T) {
	e := newTestEngine()
	e.ReviewIterations = 5
	e.PendingReviewFix = true
	e.ReviewPassed = true

	e.ResetReviewState()

	require.Equal(t, 0, e.ReviewIterations)
	require.False(t, e.PendingReviewFix)
	require.False(t, e.ReviewPassed)
}
