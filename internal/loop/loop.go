// Package loop implements the main orchestration loop.
package loop

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aymanbagabas/go-udiff"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	gitutil "github.com/alexander-akhmetov/programmator/internal/git"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/source"
	"github.com/alexander-akhmetov/programmator/internal/timing"
)

type Result struct {
	ExitReason        safety.ExitReason
	ExitMessage       string // Human-readable explanation of exit reason
	Iterations        int
	TotalFilesChanged []string
	FinalStatus       *parser.ParsedStatus
	Duration          time.Duration
	RecentSummaries   []string // Summaries from recent iterations (for debugging stagnation)
}

type OutputCallback func(text string)
type StateCallback func(state *safety.State, workItem *domain.WorkItem, filesChanged []string)
type ProcessStatsCallback func(pid int, memoryKB int64)

// EventCallback receives typed events from the loop and review runner.
// When set, the loop emits structured events instead of marker-prefixed strings.
type EventCallback func(event.Event)

// GitWorkflowConfig holds configuration for automatic git operations.
type GitWorkflowConfig struct {
	AutoCommit         bool   // Auto-commit after each phase completion
	MoveCompletedPlans bool   // Move completed plans to completed/ directory
	CompletedPlansDir  string // Directory for completed plans (default: plans/completed)
	BranchPrefix       string // Prefix for auto-created branches (default: programmator/)
	AutoBranch         bool   // Auto-create branch on start
}

type Loop struct {
	config         safety.Config
	workingDir     string
	onOutput       OutputCallback
	onEvent        EventCallback
	onStateChange  StateCallback
	onProcessStats ProcessStatsCallback
	streaming      bool
	cancelFunc     context.CancelFunc
	source         source.Source
	invoker        llm.Invoker

	stopRequested atomic.Bool

	currentState    *safety.State
	currentWorkItem *domain.WorkItem

	// Engine for pure decision logic.
	engine Engine

	// Review configuration
	reviewConfig     review.Config
	reviewOnly       bool
	reviewRunner     *review.Runner
	lastReviewIssues string // formatted issues from last review for Claude to fix

	// Prompt builder (uses customizable templates)
	promptBuilder *prompt.Builder

	// Ticket CLI command name
	ticketCommand string

	// Git workflow configuration
	gitConfig GitWorkflowConfig
	gitRepo   *gitutil.Repo

	// Executor configuration for the factory
	executorConfig llm.ExecutorConfig

	// Track consecutive invocation failures to exit early on persistent errors
	consecutiveInvokeErrors int
}

// SetSource sets the source for the loop (for testing).
func (l *Loop) SetSource(src source.Source) {
	l.source = src
}

func New(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool) *Loop {
	return NewWithSource(config, workingDir, onOutput, onStateChange, streaming, nil)
}

func NewWithSource(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool, src source.Source) *Loop {
	return &Loop{
		config:        config,
		workingDir:    workingDir,
		onOutput:      onOutput,
		onStateChange: onStateChange,
		streaming:     streaming,
		source:        src,
		reviewConfig:  review.ConfigFromEnv(),
		engine: Engine{
			SafetyConfig: config,
		},
	}
}

// SetReviewConfig sets the review configuration.
func (l *Loop) SetReviewConfig(cfg review.Config) {
	l.reviewConfig = cfg
	l.engine.MaxReviewIter = cfg.MaxIterations
}

// SetReviewOnly enables review-only mode (skips task phases).
func (l *Loop) SetReviewOnly(reviewOnly bool) {
	l.reviewOnly = reviewOnly
	l.engine.ReviewOnly = reviewOnly
}

// SetPromptBuilder sets a custom prompt builder (for customizable templates).
func (l *Loop) SetPromptBuilder(builder *prompt.Builder) {
	l.promptBuilder = builder
}

// SetTicketCommand sets the ticket CLI command name.
func (l *Loop) SetTicketCommand(cmd string) {
	l.ticketCommand = cmd
}

// SetGitWorkflowConfig sets the git workflow configuration.
func (l *Loop) SetGitWorkflowConfig(cfg GitWorkflowConfig) {
	l.gitConfig = cfg
}

// SetExecutorConfig sets the executor configuration for the invoker factory.
func (l *Loop) SetExecutorConfig(cfg llm.ExecutorConfig) {
	l.executorConfig = cfg
}

// isClaudeExecutor returns true when the configured executor is Claude (or default).
func (l *Loop) isClaudeExecutor() bool {
	return l.executorConfig.Name == "claude" || l.executorConfig.Name == ""
}

// executorName returns a display name for the configured executor.
func (l *Loop) executorName() string {
	if l.executorConfig.Name == "" {
		return "claude"
	}
	return l.executorConfig.Name
}

// setupGitWorkflow initializes the git repo and optionally creates a branch.
func (l *Loop) setupGitWorkflow(sourceID string, isPlan bool) error {
	// Initialize git repo
	repo, err := gitutil.NewRepo(l.workingDir)
	if err != nil {
		return fmt.Errorf("open git repo: %w", err)
	}
	l.gitRepo = repo

	// Only create branch if auto-branch is enabled
	if !l.gitConfig.AutoBranch {
		return nil
	}

	// Generate branch name
	prefix := l.gitConfig.BranchPrefix
	if prefix == "" {
		prefix = "programmator/"
	}

	branchName := gitutil.BranchNameFromSource(sourceID, isPlan)
	if !strings.HasPrefix(branchName, prefix) {
		// BranchNameFromSource already adds "programmator/", replace with configured prefix
		branchName = prefix + strings.TrimPrefix(branchName, "programmator/")
	}

	// Create or checkout the branch
	l.log(fmt.Sprintf("Setting up branch: %s", branchName))

	if err := l.gitRepo.CreateBranch(branchName); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	return nil
}

// autoCommitPhase commits changes after a phase is completed.
func (l *Loop) autoCommitPhase(phaseName string, filesChanged []string) error {
	if !l.gitConfig.AutoCommit || l.gitRepo == nil || len(filesChanged) == 0 {
		return nil
	}

	l.log(fmt.Sprintf("Auto-committing: %s", phaseName))

	if err := l.gitRepo.AddAndCommit(filesChanged, phaseName); err != nil {
		return fmt.Errorf("auto-commit: %w", err)
	}

	return nil
}

// moveCompletedPlan moves a completed plan file to the completed directory.
func (l *Loop) moveCompletedPlan(rc *runContext) error {
	if !l.gitConfig.MoveCompletedPlans {
		return nil
	}

	// Only move sources that implement Mover (e.g. plan files, not tickets)
	mover, ok := rc.source.(source.Mover)
	if !ok {
		return nil
	}

	// Determine destination directory
	destDir := l.gitConfig.CompletedPlansDir
	if destDir == "" {
		// Default: plans/completed relative to working directory
		destDir = filepath.Join(l.workingDir, "plans", "completed")
	} else if !filepath.IsAbs(destDir) {
		// Make relative paths relative to working directory
		destDir = filepath.Join(l.workingDir, destDir)
	}

	// Capture original path before move for staging the deletion
	origPath := mover.FilePath()

	// Move the plan
	newPath, err := mover.MoveTo(destDir)
	if err != nil {
		return fmt.Errorf("move plan: %w", err)
	}

	l.log(fmt.Sprintf("Moved completed plan to: %s", newPath))

	// If auto-commit is enabled, commit the move
	if l.gitConfig.AutoCommit && l.gitRepo != nil {
		// Stage the new file and the deletion of the original so the
		// commit records the move (git add <new> alone leaves the old
		// path as an unstaged deletion).
		relOrig, relOrigErr := filepath.Rel(l.workingDir, origPath)
		relNew, relNewErr := filepath.Rel(l.workingDir, newPath)
		stagingOK := true
		if relOrigErr != nil || relNewErr != nil {
			l.log(fmt.Sprintf("Warning: failed to get relative paths for plan move: orig=%v, new=%v", relOrigErr, relNewErr))
			stagingOK = false
		} else {
			if addErr := l.gitRepo.Add(relNew); addErr != nil {
				l.log(fmt.Sprintf("Warning: failed to stage new plan path: %v", addErr))
				stagingOK = false
			}
			if rmErr := l.gitRepo.Remove(relOrig); rmErr != nil {
				l.log(fmt.Sprintf("Warning: failed to stage plan deletion: %v", rmErr))
				stagingOK = false
			}
		}

		if !stagingOK {
			l.log("Warning: skipping commit due to staging failures")
		} else {
			commitMsg := "chore: move completed plan to completed/"
			if err := l.gitRepo.Commit(commitMsg); err != nil {
				l.log(fmt.Sprintf("Warning: failed to commit plan move: %v", err))
			} else {
				l.log("Committed plan move")
			}
		}
	}

	return nil
}

// loopAction indicates what the main loop should do next.
type loopAction int

const (
	loopContinue loopAction = iota
	loopReturn
	loopBreakToClaudeInvocation
	loopRetryReview
)

// runContext holds mutable state for a single Run invocation.
type runContext struct {
	ctx                context.Context
	workItemID         string
	source             source.Source
	state              *safety.State
	result             *Result
	filesChangedSet    map[string]struct{}
	workItem           *domain.WorkItem
	iterationSummaries []string // Track summaries for each iteration
	taskCompleted      bool     // Claude reported DONE for the task
}

// checkStopRequested checks if stop was requested and handles the response.
// Returns loopReturn if we should exit, loopContinue otherwise.
func (l *Loop) checkStopRequested(rc *runContext) loopAction {
	if l.stopRequested.Load() {
		l.log("Stop requested by user")
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: Stopped by user after %d iterations", rc.state.Iteration))
		rc.result.ExitReason = safety.ExitReasonUserInterrupt
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}
	return loopContinue
}

// checkContextCanceled checks if context was canceled.
// Returns loopReturn if we should exit, loopContinue otherwise.
func (l *Loop) checkContextCanceled(rc *runContext) loopAction {
	select {
	case <-rc.ctx.Done():
		rc.result.ExitReason = safety.ExitReasonUserInterrupt
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	default:
		return loopContinue
	}
}

// handleAllPhasesComplete handles the logic when the task is complete.
// Returns loopReturn if we should exit, loopBreakToClaudeInvocation if we should invoke Claude,
// loopRetryReview to re-run review without invoking Claude, or loopContinue to proceed normally.
func (l *Loop) handleAllPhasesComplete(rc *runContext) loopAction {
	taskComplete := rc.taskCompleted || rc.workItem.AllPhasesComplete()
	if !taskComplete && !l.reviewOnly {
		return loopContinue
	}

	// If we have pending review fixes, invoke Claude to fix them
	if l.engine.PendingReviewFix {
		l.log("Pending review fixes - invoking executor to fix issues")
		return loopBreakToClaudeInvocation
	}

	// Check if we should run review
	if !l.engine.ReviewPassed {
		return l.handleReview(rc)
	}

	// No review needed or already passed
	return l.completeAllPhases(rc)
}

func countReviewErrors(results []*review.Result) int {
	errorCount := 0
	for _, res := range results {
		if res.Error != nil {
			errorCount++
		}
	}
	return errorCount
}

// handleReview runs a single review iteration and decides next steps.
func (l *Loop) handleReview(rc *runContext) loopAction {
	if len(l.reviewConfig.Agents) == 0 {
		err := fmt.Errorf("review enabled but no review agents configured (review.agents)")
		l.log(err.Error())
		l.addNote(rc, fmt.Sprintf("error: %s", err.Error()))
		rc.result.ExitReason = safety.ExitReasonError
		rc.result.ExitMessage = err.Error()
		return loopReturn
	}

	// Check iteration limit before starting a new review+fix cycle.
	if l.engine.MaxReviewIter > 0 && l.engine.ReviewIterations >= l.engine.MaxReviewIter {
		l.log(fmt.Sprintf("Review iteration limit reached (%d/%d) - completing",
			l.engine.ReviewIterations, l.engine.MaxReviewIter))
		l.addNote(rc, fmt.Sprintf("warning: Review iteration limit reached (%d)",
			l.engine.MaxReviewIter))
		rc.state.ExitReviewPhase()
		return l.completeAllPhases(rc)
	}
	l.engine.ReviewIterations++

	l.log(fmt.Sprintf("Review iteration %d/%d",
		l.engine.ReviewIterations, l.engine.MaxReviewIter))

	rc.state.EnterReviewPhase()
	if l.reviewRunner == nil {
		l.applySettingsToReviewConfig()
		l.applyReviewContext(rc.workItem)
		var outputCallback review.OutputCallback
		if l.onOutput != nil && l.onEvent == nil {
			outputCallback = func(text string) {
				l.onOutput(text)
			}
		}
		l.reviewRunner = review.NewRunner(l.reviewConfig, outputCallback)
		if l.onEvent != nil {
			l.reviewRunner.SetEventCallback(event.Handler(l.onEvent))
		}
	}

	reviewResult, err := l.reviewRunner.RunIteration(rc.ctx, l.workingDir, rc.result.TotalFilesChanged)
	if err != nil {
		l.log(fmt.Sprintf("Review error: %v", err))
		l.addNote(rc, fmt.Sprintf("error: Review failed: %v", err))
		rc.result.ExitReason = safety.ExitReasonError
		rc.result.ExitMessage = err.Error()
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}

	rc.state.RecordReviewIteration()

	errorCount := countReviewErrors(reviewResult.Results)
	if errorCount > 0 {
		// Agent errors count as stagnation (no progress made)
		rc.state.ConsecutiveNoChanges++

		// Check if stagnation limit exceeded
		checkResult := safety.Check(l.config, rc.state)
		if checkResult.ShouldExit {
			l.log(fmt.Sprintf("Review agent errors (%d) - %s", errorCount, checkResult.Message))
			l.addNote(rc, fmt.Sprintf("error: Review agent errors - %s", checkResult.Message))
			rc.result.ExitReason = checkResult.Reason
			rc.result.ExitMessage = checkResult.Message
			rc.result.Iterations = rc.state.Iteration
			l.engine.ReviewIterations--
			return loopReturn
		}

		l.log(fmt.Sprintf("Review agent errors (%d) - retrying review without invoking Claude", errorCount))
		l.addNote(rc, fmt.Sprintf("warning: Review agent errors (%d) - retrying review", errorCount))

		l.engine.ReviewIterations--
		l.engine.PendingReviewFix = false
		l.engine.ReviewPassed = false
		return loopRetryReview
	}

	// Reset stagnation counter on successful review run
	rc.state.ConsecutiveNoChanges = 0

	decision := l.engine.DecideReview(reviewResult.Passed)

	if decision.Passed {
		l.log("Review passed - no issues found")
		l.addNote(rc, "progress: Review passed")
		rc.state.ExitReviewPhase()
		return l.completeAllPhases(rc)
	}

	issueNote := review.FormatIssuesMarkdown(reviewResult.Results)
	l.lastReviewIssues = issueNote

	// NeedsFix: invoke Claude to fix issues
	l.log(fmt.Sprintf("Review found %d issues", reviewResult.TotalIssues))
	l.addNote(rc, fmt.Sprintf("review: [iter %d] Found %d issues:\n%s",
		l.engine.ReviewIterations, reviewResult.TotalIssues, issueNote))

	return loopBreakToClaudeInvocation
}

// completeAllPhases marks the work item as complete and returns.
func (l *Loop) completeAllPhases(rc *runContext) loopAction {
	l.log("All phases complete!")
	_ = rc.source.SetStatus(rc.workItemID, protocol.WorkItemClosed)
	_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: Completed all phases in %d iterations", rc.state.Iteration))

	// Move completed plan if configured
	if err := l.moveCompletedPlan(rc); err != nil {
		l.log(fmt.Sprintf("Warning: failed to move completed plan: %v", err))
	}

	rc.result.ExitReason = safety.ExitReasonComplete
	rc.result.Iterations = rc.state.Iteration
	return loopReturn
}

// processClaudeStatus processes the status returned by Claude.
// Returns loopReturn if we should exit, loopContinue otherwise.
func (l *Loop) processClaudeStatus(rc *runContext, status *parser.ParsedStatus) loopAction {
	l.log(fmt.Sprintf("Status: %s", status.Status))
	l.log(fmt.Sprintf("Summary: %s", status.Summary))

	rc.result.FinalStatus = status
	l.recordPhaseProgress(rc, status)
	l.trackFilesChanged(rc, status)

	// Track iteration summary for stagnation debugging
	rc.iterationSummaries = append(rc.iterationSummaries,
		FormatIterationSummary(rc.state.Iteration, status.Summary, status.FilesChanged))

	rc.state.RecordIteration(status.FilesChanged, status.Error)

	// Use engine to process status
	result := l.engine.ProcessStatus(ProcessStatusInput{
		Status:           status,
		Iteration:        rc.state.Iteration,
		PendingReviewFix: l.engine.PendingReviewFix,
	})

	if result.ResetPendingReviewFix {
		l.engine.PendingReviewFix = false
		l.lastReviewIssues = ""
	}

	if l.onStateChange != nil {
		l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
	}

	if result.TaskCompleted {
		l.log("Executor reported DONE")
		rc.taskCompleted = true
		if !rc.state.InReviewPhase {
			l.addNote(rc, fmt.Sprintf("progress: Task marked complete in %d iterations", rc.state.Iteration))
		}
		return loopContinue
	}

	if result.ShouldExit {
		l.log(fmt.Sprintf("Executor reported BLOCKED: %s", result.BlockedError))
		l.addNote(rc, fmt.Sprintf("error: [iter %d] BLOCKED: %s", rc.state.Iteration, result.BlockedError))
		rc.result.ExitReason = result.ExitReason
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}

	return loopContinue
}

// recordPhaseProgress records phase completion or progress notes.
func (l *Loop) recordPhaseProgress(rc *runContext, status *parser.ParsedStatus) {
	if status.PhaseCompleted != "" {
		l.log(fmt.Sprintf("Phase completed: %s", status.PhaseCompleted))
		if err := rc.source.UpdatePhase(rc.workItemID, status.PhaseCompleted); err != nil {
			l.log(fmt.Sprintf("Warning: failed to update phase '%s': %v", status.PhaseCompleted, err))
		}
		l.addNote(rc, fmt.Sprintf("progress: [iter %d] Completed %s", rc.state.Iteration, status.PhaseCompleted))

		// Auto-commit after phase completion if enabled
		if err := l.autoCommitPhase(status.PhaseCompleted, status.FilesChanged); err != nil {
			l.log(fmt.Sprintf("Warning: auto-commit failed: %v", err))
		}
	} else {
		l.addNote(rc, fmt.Sprintf("progress: [iter %d] %s", rc.state.Iteration, status.Summary))
	}
}

// trackFilesChanged records which files were changed.
func (l *Loop) trackFilesChanged(rc *runContext, status *parser.ParsedStatus) {
	if len(status.FilesChanged) > 0 {
		l.log(fmt.Sprintf("Files changed: %s", strings.Join(status.FilesChanged, ", ")))
		for _, f := range status.FilesChanged {
			if _, exists := rc.filesChangedSet[f]; !exists {
				rc.filesChangedSet[f] = struct{}{}
				rc.result.TotalFilesChanged = append(rc.result.TotalFilesChanged, f)
			}
		}
	}
}

func (l *Loop) Run(workItemID string) (*Result, error) {
	timing.Log("Loop.Run: start")
	startTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFunc = cancel
	defer cancel()

	timing.Log("Loop.Run: creating source")
	src := l.source
	if src == nil {
		// Auto-detect source type based on workItemID
		src, workItemID = source.Detect(workItemID, l.ticketCommand)
	}
	timing.Log("Loop.Run: source created")

	result := &Result{
		ExitReason:        safety.ExitReasonComplete,
		TotalFilesChanged: make([]string, 0),
	}
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	timing.Log("Loop.Run: fetching work item")
	workItem, err := src.Get(workItemID)
	timing.Log("Loop.Run: work item fetched")
	if err != nil {
		result.ExitReason = safety.ExitReasonError
		return result, err
	}

	l.log(fmt.Sprintf("Starting on %s %s: %s", src.Type(), workItemID, workItem.Title))

	// Validate review config before changing ticket state
	if len(l.reviewConfig.Agents) == 0 {
		err := fmt.Errorf("review enabled but no review agents configured (review.agents)")
		l.log(err.Error())
		result.ExitReason = safety.ExitReasonError
		result.ExitMessage = err.Error()
		return result, err
	}

	_ = src.SetStatus(workItemID, protocol.WorkItemInProgress)

	// Set up git repo and optionally create branch
	if err := l.setupGitWorkflow(workItemID, src.Type() == protocol.SourceTypePlan); err != nil {
		l.log(fmt.Sprintf("Warning: git workflow setup failed: %v", err))
	}

	rc := &runContext{
		ctx:             ctx,
		workItemID:      workItemID,
		source:          src,
		state:           safety.NewState(),
		result:          result,
		filesChangedSet: make(map[string]struct{}),
		workItem:        workItem,
	}

	if l.onStateChange != nil {
		l.onStateChange(rc.state, rc.workItem, nil)
	}

	for {
		if action := l.checkStopRequested(rc); action == loopReturn {
			return rc.result, nil
		}

		if action := l.checkContextCanceled(rc); action == loopReturn {
			return rc.result, nil
		}

		rc.workItem, err = rc.source.Get(rc.workItemID)
		if err != nil {
			rc.result.ExitReason = safety.ExitReasonError
			return rc.result, err
		}

		action := l.handleAllPhasesComplete(rc)
		if action == loopReturn {
			return rc.result, nil
		}
		if action == loopRetryReview {
			continue
		}
		// If action == loopBreakToClaudeInvocation, we fall through to invoke Claude

		rc.state.Iteration++

		checkResult := safety.Check(l.config, rc.state)
		if checkResult.ShouldExit {
			l.log(fmt.Sprintf("Safety exit: %s", checkResult.Reason))
			l.addNote(rc, fmt.Sprintf("error: Safety exit after %d iters: %s", rc.state.Iteration, checkResult.Reason))
			rc.result.ExitReason = checkResult.Reason
			rc.result.ExitMessage = checkResult.Message
			rc.result.Iterations = rc.state.Iteration
			rc.result.RecentSummaries = l.getRecentSummaries(rc, 5)
			return rc.result, nil
		}

		currentPhase := rc.workItem.CurrentPhase()
		l.logIterationSeparator(rc.state.Iteration, l.config.MaxIterations)
		l.log(fmt.Sprintf("Iteration %d/%d", rc.state.Iteration, l.config.MaxIterations))
		if currentPhase != nil {
			l.log(fmt.Sprintf("Current phase: %s", currentPhase.Name))
		}

		var promptText string
		if l.engine.PendingReviewFix && l.promptBuilder != nil {
			// Use review fix prompt with the stored issues so review templates apply
			var promptErr error
			promptText, promptErr = l.promptBuilder.BuildReviewFirst("", rc.result.TotalFilesChanged, l.lastReviewIssues, l.engine.ReviewIterations, l.gitConfig.AutoCommit)
			if promptErr != nil {
				l.log(fmt.Sprintf("Failed to build review fix prompt: %v, falling back to task prompt", promptErr))
				promptText = prompt.Build(rc.workItem)
			}
		} else if l.promptBuilder != nil {
			var promptErr error
			promptText, promptErr = l.promptBuilder.Build(rc.workItem)
			if promptErr != nil {
				l.log(fmt.Sprintf("Failed to build prompt from templates: %v, falling back to defaults", promptErr))
				promptText = prompt.Build(rc.workItem)
			}
		} else {
			promptText = prompt.Build(rc.workItem)
		}

		l.currentState = rc.state
		l.currentWorkItem = rc.workItem

		if l.onStateChange != nil {
			l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
		}

		l.log(fmt.Sprintf("Invoking %s...", l.executorName()))

		output, err := l.invokeClaudePrint(ctx, promptText)
		if err != nil {
			l.log(fmt.Sprintf("Invocation failed: %v", err))
			rc.state.RecordIteration(nil, "invocation_error")
			if l.onStateChange != nil {
				l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
			}
			l.consecutiveInvokeErrors++
			if l.consecutiveInvokeErrors >= 3 {
				l.log("3 consecutive invocation failures — exiting")
				rc.result.ExitReason = safety.ExitReasonError
				rc.result.ExitMessage = fmt.Sprintf("3 consecutive invocation failures, last: %v", err)
				rc.result.Iterations = rc.state.Iteration
				return rc.result, nil
			}
			continue
		}
		l.consecutiveInvokeErrors = 0

		status, err := parser.Parse(output)
		if err != nil {
			rc.result.ExitReason = safety.ExitReasonError
			return rc.result, err
		}

		if status == nil {
			l.log("Warning: No " + protocol.StatusBlockKey + " found in output")
			rc.state.RecordIteration(nil, "no_status_block")
			if l.onStateChange != nil {
				l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
			}
			continue
		}

		if action := l.processClaudeStatus(rc, status); action == loopReturn {
			return rc.result, nil
		}
	}
}

// invokeClaudePrint invokes Claude via the llm.Invoker interface.
// It wires loop-specific callbacks (output formatting, token tracking,
// process stats) into InvokeOptions.
func (l *Loop) invokeClaudePrint(ctx context.Context, promptText string) (string, error) {
	inv := l.invoker
	if inv == nil {
		var err error
		inv, err = llm.NewInvoker(l.executorConfig)
		if err != nil {
			return "", fmt.Errorf("create invoker: %w", err)
		}
		l.invoker = inv
	}

	opts := llm.InvokeOptions{
		WorkingDir: l.workingDir,
		Streaming:  l.streaming,
		ExtraFlags: l.executorConfig.ExtraFlags,
		Timeout:    l.config.Timeout,
		OnOutput: func(text string) {
			l.emit(event.Markdown(text))
			if l.onOutput != nil && l.onEvent == nil {
				l.onOutput(text)
			}
		},
		OnToolUse: func(name string, input any) {
			l.outputToolUse(name, input)
		},
		OnToolResult: func(toolName, result string) {
			l.handleToolResult(toolName, result)
		},
		OnSystemInit: func(model string) {
			if l.currentState != nil {
				l.currentState.Model = model
				l.notifyStateChange()
			}
		},
		OnTokens: func(inputTokens, outputTokens int) {
			if l.currentState != nil {
				l.currentState.SetCurrentIterTokens(inputTokens, outputTokens)
				l.notifyStateChange()
			}
		},
		OnFinalTokens: func(model string, inputTokens, outputTokens int) {
			if l.currentState != nil {
				l.currentState.FinalizeIterTokens(model, inputTokens, outputTokens)
				l.notifyStateChange()
			}
		},
	}

	if l.onProcessStats != nil {
		stopStats := make(chan struct{})
		var stopOnce sync.Once
		closeStats := func() {
			stopOnce.Do(func() {
				close(stopStats)
			})
		}
		opts.OnProcessStart = func(pid int) {
			go l.pollProcessStats(pid, stopStats)
		}
		opts.OnProcessEnd = func() {
			closeStats()
			l.onProcessStats(0, 0) // Signal process ended
		}
		defer closeStats() // ensure goroutine stops even if Invoke errors before OnProcessEnd
	}

	res, err := inv.Invoke(ctx, promptText, opts)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func (l *Loop) handleToolResult(toolName, result string) {
	if (l.onOutput == nil && l.onEvent == nil) || toolName == "" {
		return
	}

	summary := formatToolResultSummary(toolName, result)
	if summary != "" {
		l.emit(event.ToolResult(fmt.Sprintf("  ⎿  %s", summary)))
		if l.onOutput != nil && l.onEvent == nil {
			l.onOutput(fmt.Sprintf(protocol.MarkerToolRes+"  ⎿  %s\n", summary))
		}
	}
}

func formatToolResultSummary(toolName, result string) string {
	if result == "" {
		return ""
	}

	lines := strings.Split(result, "\n")
	lineCount := len(lines)
	if lineCount > 0 && lines[lineCount-1] == "" {
		lineCount--
	}

	switch toolName {
	case "Read":
		return fmt.Sprintf("Read %d lines", lineCount)
	case "Glob":
		fileCount := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				fileCount++
			}
		}
		if fileCount == 0 {
			return "No files found"
		}
		return fmt.Sprintf("Found %d files", fileCount)
	case "Grep":
		matchCount := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				matchCount++
			}
		}
		if matchCount == 0 {
			return "No matches found"
		}
		return fmt.Sprintf("Found %d matches", matchCount)
	case "Bash":
		if lineCount == 0 {
			return "(no output)"
		}
		firstLine := strings.TrimSpace(lines[0])
		if len(firstLine) > 60 {
			firstLine = firstLine[:57] + "..."
		}
		if lineCount == 1 {
			return firstLine
		}
		return fmt.Sprintf("%s (+%d more lines)", firstLine, lineCount-1)
	case "Write":
		return "File written"
	case "Edit":
		return "File updated"
	default:
		if lineCount <= 1 {
			return strings.TrimSpace(result)
		}
		return fmt.Sprintf("%d lines", lineCount)
	}
}

func (l *Loop) outputToolUse(name string, input any) {
	if l.onOutput == nil && l.onEvent == nil {
		return
	}
	toolLine := name
	inputMap, hasInput := input.(map[string]any)
	if hasInput {
		toolLine += formatToolArg(name, inputMap)
	}
	l.emit(event.ToolUse(toolLine))
	if l.onOutput != nil && l.onEvent == nil {
		l.onOutput(fmt.Sprintf("\n"+protocol.MarkerTool+"%s\n", toolLine))
	}

	// Show diff for Edit operations
	if name == "Edit" && hasInput {
		l.outputEditDiff(inputMap)
	}
}

func (l *Loop) outputEditDiff(input map[string]any) {
	oldStr, oldOk := input["old_string"].(string)
	newStr, newOk := input["new_string"].(string)
	if !oldOk || !newOk {
		return
	}

	// Count lines changed
	oldLines := strings.Count(oldStr, "\n")
	newLines := strings.Count(newStr, "\n")
	if !strings.HasSuffix(oldStr, "\n") && oldStr != "" {
		oldLines++
	}
	if !strings.HasSuffix(newStr, "\n") && newStr != "" {
		newLines++
	}

	// Build summary like Claude Code: "Added X lines, removed Y lines"
	var parts []string
	added := newLines - oldLines
	if added > 0 {
		parts = append(parts, fmt.Sprintf("Added %d line", added))
		if added > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	removed := oldLines - newLines
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("removed %d line", removed))
		if removed > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	if added == 0 && removed == 0 && oldStr != newStr {
		parts = append(parts, fmt.Sprintf("Modified %d line", oldLines))
		if oldLines > 1 {
			parts[len(parts)-1] += "s"
		}
	}

	if len(parts) > 0 {
		hunkText := fmt.Sprintf("  ⎿  %s", strings.Join(parts, ", "))
		l.emit(event.DiffHunk(hunkText))
		if l.onOutput != nil && l.onEvent == nil {
			l.onOutput(fmt.Sprintf(protocol.MarkerDiffAt+"%s\n", hunkText))
		}
	}

	// Generate unified diff for the actual changes
	diff := udiff.Unified("old", "new", oldStr, newStr)
	if diff == "" {
		return
	}

	// Output only the changed lines (skip headers, hunks, and context)
	for line := range strings.SplitSeq(diff, "\n") {
		if line == "" {
			continue
		}
		lineText := fmt.Sprintf("      %s", line)
		switch {
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "@@"):
			// Skip file headers and hunk markers
			continue
		case strings.HasPrefix(line, "-"):
			l.emit(event.DiffDel(lineText))
			if l.onOutput != nil && l.onEvent == nil {
				l.onOutput(fmt.Sprintf(protocol.MarkerDiffDel+"%s\n", lineText))
			}
		case strings.HasPrefix(line, "+"):
			l.emit(event.DiffAdd(lineText))
			if l.onOutput != nil && l.onEvent == nil {
				l.onOutput(fmt.Sprintf(protocol.MarkerDiffAdd+"%s\n", lineText))
			}
		default:
			// Context lines - show them dimmed for context
			l.emit(event.DiffCtx(lineText))
			if l.onOutput != nil && l.onEvent == nil {
				l.onOutput(fmt.Sprintf(protocol.MarkerDiffCtx+"%s\n", lineText))
			}
		}
	}
}

func (l *Loop) notifyStateChange() {
	if l.onStateChange != nil && l.currentWorkItem != nil {
		l.onStateChange(l.currentState, l.currentWorkItem, nil)
	}
}

func formatToolArg(toolName string, input map[string]any) string {
	switch toolName {
	case "Read", "Write", "Edit":
		if path, ok := input["file_path"].(string); ok {
			return " " + path
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			if len(cmd) > 80 {
				cmd = cmd[:80] + "..."
			}
			return " " + cmd
		}
	case "Glob":
		if pattern, ok := input["pattern"].(string); ok {
			return " " + pattern
		}
	case "Grep":
		if pattern, ok := input["pattern"].(string); ok {
			return " " + pattern
		}
	case "Task":
		if desc, ok := input["description"].(string); ok {
			return " " + desc
		}
	}
	return ""
}

func (l *Loop) Stop() {
	l.stopRequested.Store(true)
	if l.cancelFunc != nil {
		l.cancelFunc()
	}
}

func (l *Loop) log(message string) {
	l.emit(event.Prog(message))
	if l.onOutput != nil && l.onEvent == nil {
		l.onOutput(fmt.Sprintf("\n"+protocol.MarkerProg+"%s\n", message))
	}
}

func (l *Loop) logIterationSeparator(iteration, maxIterations int) {
	separator := fmt.Sprintf("\n\n---\n\n### 🔄 Iteration %d/%d\n\n", iteration, maxIterations)
	l.emit(event.IterationSeparator(separator))
	if l.onOutput != nil && l.onEvent == nil {
		l.onOutput(separator)
	}
}

func (r *Result) FilesChangedList() []string {
	return r.TotalFilesChanged
}

// getRecentSummaries returns the last n iteration summaries for debugging.
func (l *Loop) getRecentSummaries(rc *runContext, n int) []string {
	if len(rc.iterationSummaries) <= n {
		return rc.iterationSummaries
	}
	return rc.iterationSummaries[len(rc.iterationSummaries)-n:]
}

// addNote adds a note to the work item, ignoring errors.
func (l *Loop) addNote(rc *runContext, note string) {
	_ = rc.source.AddNote(rc.workItemID, note)
}

func (l *Loop) pollProcessStats(pid int, stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			memKB := getProcessMemory(pid)
			if l.onProcessStats != nil {
				l.onProcessStats(pid, memKB)
			}
		}
	}
}

func getProcessMemory(pid int) int64 {
	cmd := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	rss, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return rss
}

// SetInvoker sets the llm.Invoker used for Claude invocations.
func (l *Loop) SetInvoker(inv llm.Invoker) {
	l.invoker = inv
}

func (l *Loop) SetProcessStatsCallback(cb ProcessStatsCallback) {
	l.onProcessStats = cb
}

// SetEventCallback sets the typed event handler. When set, the loop emits
// structured events for prog/tool/review/diff messages instead of (or in
// addition to) the legacy marker-prefixed strings on OutputCallback.
func (l *Loop) SetEventCallback(cb EventCallback) {
	l.onEvent = cb
}

// emit sends a typed event to the event callback, if set.
func (l *Loop) emit(e event.Event) {
	if l.onEvent != nil {
		l.onEvent(e)
	}
}

// applySettingsToReviewConfig copies config settings into the review config.
func (l *Loop) applySettingsToReviewConfig() {
	if l.isClaudeExecutor() {
		if len(l.executorConfig.ExtraFlags) > 0 {
			l.reviewConfig.ClaudeFlags = strings.Join(l.executorConfig.ExtraFlags, " ")
		}
		l.reviewConfig.EnvConfig = l.executorConfig.Claude
	}
}

func (l *Loop) applyReviewContext(workItem *domain.WorkItem) {
	if workItem == nil {
		return
	}
	l.reviewConfig.TicketContext = workItem.RawContent
}

// SetReviewRunner sets a custom review runner (useful for testing).
func (l *Loop) SetReviewRunner(runner *review.Runner) {
	l.reviewRunner = runner
}
