package loop

import (
	"fmt"
	"strings"

	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/safety"
)

// Engine makes pure decisions about what the loop should do next.
// It holds no I/O references—only configuration and transient review state.
type Engine struct {
	SafetyConfig safety.Config

	// Review state (mutable, updated by the runner after each decision).
	ReviewIterations int  // total review iterations completed
	PendingReviewFix bool // true when Claude should fix review issues
	ReviewPassed     bool // true when review has passed
	ReviewOnly       bool // true for review-only mode (skips task phases)
	MaxReviewIter    int  // from review.max_iterations; 0 means unlimited
}

// ProcessStatus analyses a parsed Claude status block and returns pure decisions.
func (e *Engine) ProcessStatus(input ProcessStatusInput) StatusProcessResult {
	status := input.Status
	if status == nil {
		return StatusProcessResult{}
	}
	result := StatusProcessResult{
		PhaseCompleted: status.PhaseCompleted,
		FilesChanged:   status.FilesChanged,
		Summary:        status.Summary,
	}

	if input.PendingReviewFix {
		result.ResetPendingReviewFix = true
	}

	switch status.Status {
	case protocol.StatusDone:
		result.TaskCompleted = true

	case protocol.StatusBlocked:
		result.Blocked = true
		result.BlockedError = status.Error
		result.ExitReason = safety.ExitReasonBlocked
		result.ShouldExit = true
	}

	return result
}

// DecideReview evaluates the review result and decides what to do next.
// The iteration limit is now checked before running the review (in handleReview),
// so DecideReview only distinguishes between passed and needs-fix.
func (e *Engine) DecideReview(passed bool) ReviewDecision {
	if passed {
		e.ReviewPassed = true
		return ReviewDecision{Passed: true}
	}

	e.PendingReviewFix = true
	return ReviewDecision{NeedsFix: true}
}

// FormatIterationSummary builds a summary string for debugging stagnation.
func FormatIterationSummary(iteration int, summary string, filesChanged []string) string {
	s := fmt.Sprintf("[iter %d] %s", iteration, summary)
	if len(filesChanged) > 0 {
		s += fmt.Sprintf(" (files: %s)", strings.Join(filesChanged, ", "))
	} else {
		s += " (no files changed)"
	}
	return s
}

// ResetReviewState resets review tracking for a fresh run.
func (e *Engine) ResetReviewState() {
	e.ReviewIterations = 0
	e.PendingReviewFix = false
	e.ReviewPassed = false
}
