package engine

import (
	"fmt"
	"strings"

	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/safety"
)

// Engine makes pure decisions about what the loop should do next.
// It holds no I/O references—only configuration and transient review state.
type Engine struct {
	SafetyConfig safety.Config

	// Review state (mutable, updated by the runner after each decision).
	ReviewPhaseCount int  // total number of review phases configured
	CurrentPhaseIdx  int  // index into the review phases slice
	CurrentPhaseIter int  // iterations spent on current review phase
	PendingReviewFix bool // true when Claude should fix review issues
	ReviewPassed     bool // true when all review phases have passed
	ReviewOnly       bool // true for review-only mode (skips task phases)

	// PhaseMaxIterFunc returns the max iterations for a given review phase
	// index.  Injected by the runner so the engine stays decoupled from
	// review.Config.  If nil, the phase max is treated as 0 (no retries
	// allowed—first failure exceeds the limit immediately).
	PhaseMaxIterFunc func(phaseIdx int) int
}

// DecideNext determines the next action at the top of the main loop iteration,
// after the work item has been fetched and before Claude is invoked.
//
// Inputs:
//   - stopped: user requested stop
//   - ctxDone: context was cancelled
//   - workItem: the current work item (just fetched)
//   - taskCompleted: Claude reported DONE in a previous iteration
//   - state: safety state for limit checks
func (e *Engine) DecideNext(stopped, ctxDone bool, workItem *domain.WorkItem, taskCompleted bool, state *safety.State) Action {
	// 1. User stop
	if stopped {
		return Action{
			Kind:       ActionExit,
			ExitReason: safety.ExitReasonUserInterrupt,
			Iterations: state.Iteration,
		}
	}

	// 2. Context cancellation
	if ctxDone {
		return Action{
			Kind:       ActionExit,
			ExitReason: safety.ExitReasonUserInterrupt,
			Iterations: state.Iteration,
		}
	}

	// 3. Check if all phases are complete (or review-only mode)
	allComplete := taskCompleted || (workItem != nil && workItem.AllPhasesComplete())
	if allComplete || e.ReviewOnly {
		action := e.decideOnCompletion()
		if action.Kind != ActionInvokeLLM || action.IsReviewFix {
			return action
		}
		// ActionInvokeLLM with IsReviewFix=true means fall through to invoke
		// (the runner will use the review fix prompt)
	}

	// If we're not complete and not review-only, this is a normal iteration.
	// Safety check happens in the runner before calling DecideNext, or we
	// return InvokeLLM and let the runner handle it.
	return Action{Kind: ActionInvokeLLM}
}

// decideOnCompletion handles the case when all task phases are done (or
// review-only mode). Returns the appropriate action.
func (e *Engine) decideOnCompletion() Action {
	// Pending review fix → invoke Claude to fix
	if e.PendingReviewFix {
		return Action{Kind: ActionInvokeLLM, IsReviewFix: true}
	}

	// Review not yet passed → run review
	if !e.ReviewPassed {
		return Action{Kind: ActionRunReview, ReviewPhaseIndex: e.CurrentPhaseIdx}
	}

	// All done
	return Action{Kind: ActionComplete}
}

// CheckSafety evaluates safety limits and returns the result.
func (e *Engine) CheckSafety(state *safety.State) SafetyCheckResult {
	cr := safety.Check(e.SafetyConfig, state)
	return SafetyCheckResult{
		ShouldExit:  cr.ShouldExit,
		ExitReason:  cr.Reason,
		ExitMessage: cr.Message,
	}
}

// ProcessStatus analyses a parsed Claude status block and returns pure
// decisions. The runner is responsible for all I/O side effects.
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

	// Clear pending review fix flag
	if input.PendingReviewFix {
		result.ResetPendingReviewFix = true
	}

	// Check status value
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

// DecideReview evaluates the current review phase result and decides what
// to do next.
//
// Parameters:
//   - passed: whether the review phase found no issues
func (e *Engine) DecideReview(passed bool) ReviewDecision {
	if e.ReviewPhaseCount == 0 {
		return ReviewDecision{ExitError: "review enabled but no review phases configured (review.phases)"}
	}

	// All phases already done?
	if e.CurrentPhaseIdx >= e.ReviewPhaseCount {
		return ReviewDecision{AllPhasesDone: true}
	}

	if passed {
		// Advance to next phase
		e.CurrentPhaseIdx++
		e.CurrentPhaseIter = 0

		if e.CurrentPhaseIdx >= e.ReviewPhaseCount {
			e.ReviewPassed = true
			return ReviewDecision{PhasePassed: true, AdvancePhase: true, AllPhasesDone: true}
		}
		return ReviewDecision{PhasePassed: true, AdvancePhase: true}
	}

	// Phase found issues
	e.CurrentPhaseIter++
	e.PendingReviewFix = true

	// Check iteration limit
	phaseMax := 0
	if e.PhaseMaxIterFunc != nil {
		phaseMax = e.PhaseMaxIterFunc(e.CurrentPhaseIdx)
	}

	if e.CurrentPhaseIter > phaseMax {
		// Exceeded limit → advance to next phase
		e.CurrentPhaseIdx++
		e.CurrentPhaseIter = 0
		e.PendingReviewFix = false

		if e.CurrentPhaseIdx >= e.ReviewPhaseCount {
			e.ReviewPassed = true
			return ReviewDecision{ExceededLimit: true, AdvancePhase: true, AllPhasesDone: true}
		}
		return ReviewDecision{ExceededLimit: true, AdvancePhase: true}
	}

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

// ResetReviewState resets review phase tracking for a fresh run.
func (e *Engine) ResetReviewState() {
	e.CurrentPhaseIdx = 0
	e.CurrentPhaseIter = 0
	e.PendingReviewFix = false
	e.ReviewPassed = false
}
