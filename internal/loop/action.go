// The engine is a pure state machine that decides the next action
// for the main orchestration loop. It receives inputs (work item state,
// parsed status, safety checks, user stop signals) and returns Action values
// describing side effects for the runner to execute.
package loop

import (
	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// ActionKind identifies the type of action the runner should execute.
type ActionKind int

const (
	// ActionInvokeLLM tells the runner to build a prompt and invoke the LLM.
	ActionInvokeLLM ActionKind = iota
	// ActionRunReview tells the runner to execute the review iteration.
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

	// LLM invocation fields (ActionInvokeLLM)
	IsReviewFix bool // true when the LLM should fix review issues
}

// StatusProcessResult holds the engine's decisions after processing a parsed
// Claude status block. The runner uses these to perform I/O.
type StatusProcessResult struct {
	PhaseCompleted        string
	FilesChanged          []string
	Summary               string
	TaskCompleted         bool
	Blocked               bool
	BlockedError          string
	ExitReason            safety.ExitReason
	ShouldExit            bool
	ResetPendingReviewFix bool
}

// ReviewDecision describes what to do after a review iteration runs.
type ReviewDecision struct {
	// Passed is true if the review found no issues.
	Passed bool
	// NeedsFix is true when Claude should be invoked to fix issues.
	NeedsFix bool
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
