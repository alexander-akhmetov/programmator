// Package engine implements a pure state machine that decides the next action
// for the main orchestration loop. The engine receives inputs (work item state,
// parsed status, safety checks, user stop signals) and returns Action values
// describing side effects for the runner to execute.
package engine

import (
	"github.com/worksonmyai/programmator/internal/parser"
	"github.com/worksonmyai/programmator/internal/safety"
)

// ActionKind identifies the type of action the runner should execute.
type ActionKind int

const (
	// ActionInvokeLLM tells the runner to build a prompt and invoke the LLM.
	ActionInvokeLLM ActionKind = iota
	// ActionRunReview tells the runner to execute the current review phase.
	ActionRunReview
	// ActionComplete tells the runner that all work is done.
	ActionComplete
	// ActionExit tells the runner to exit with the given reason.
	ActionExit
)

// Action is the instruction returned by the engine to the loop runner.
type Action struct {
	Kind ActionKind

	// Exit fields (ActionExit / ActionComplete)
	ExitReason  safety.ExitReason
	ExitMessage string
	Iterations  int

	// Review fields (ActionRunReview)
	ReviewPhaseIndex int

	// LLM invocation fields (ActionInvokeLLM)
	IsReviewFix bool // true when the LLM should fix review issues
}

// StatusProcessResult holds the engine's decisions after processing a parsed
// Claude status block. The runner uses these to perform I/O.
type StatusProcessResult struct {
	// PhaseCompleted is the name of the completed phase (empty if none).
	PhaseCompleted string
	// FilesChanged lists files reported as changed.
	FilesChanged []string
	// Summary is the iteration summary.
	Summary string
	// TaskCompleted is true when Claude reported DONE.
	TaskCompleted bool
	// Blocked is true when Claude reported BLOCKED.
	Blocked bool
	// BlockedError is the error message when blocked.
	BlockedError string
	// ExitReason is set when the loop should exit (BLOCKED status).
	ExitReason safety.ExitReason
	// ShouldExit is true when the runner should return immediately.
	ShouldExit bool
	// ResetPendingReviewFix indicates the pending review fix flag should be cleared.
	ResetPendingReviewFix bool
}

// ReviewDecision describes what to do after a review phase runs.
type ReviewDecision struct {
	// PhasePassed is true if the review phase found no issues.
	PhasePassed bool
	// AdvancePhase is true when moving to the next review phase.
	AdvancePhase bool
	// NeedsFix is true when Claude should be invoked to fix issues.
	NeedsFix bool
	// ExceededLimit is true when the phase exceeded its iteration limit
	// and should advance regardless.
	ExceededLimit bool
	// AllPhasesDone is true when all review phases have passed.
	AllPhasesDone bool
	// ExitError is set when a fatal error occurs (e.g., no review phases configured).
	ExitError string
}

// SafetyCheckResult wraps the safety check outcome with exit details.
type SafetyCheckResult struct {
	ShouldExit  bool
	ExitReason  safety.ExitReason
	ExitMessage string
}

// ProcessStatusInput bundles the inputs needed by ProcessStatus.
type ProcessStatusInput struct {
	Status           *parser.ParsedStatus
	Iteration        int
	PendingReviewFix bool
}
