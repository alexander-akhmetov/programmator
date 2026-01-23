// Package loop implements the main orchestration loop.
package loop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aymanbagabas/go-udiff"

	"github.com/alexander-akhmetov/programmator/internal/debug"
	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
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
type StateCallback func(state *safety.State, workItem *source.WorkItem, filesChanged []string)
type ProcessStatsCallback func(pid int, memoryKB int64)
type ClaudeInvoker func(ctx context.Context, promptText string) (string, error)

type Loop struct {
	config               safety.Config
	workingDir           string
	onOutput             OutputCallback
	onStateChange        StateCallback
	onProcessStats       ProcessStatsCallback
	streaming            bool
	cancelFunc           context.CancelFunc
	source               source.Source
	claudeInvoker        ClaudeInvoker
	permissionSocketPath string

	mu            sync.Mutex
	paused        bool
	stopRequested bool
	pauseCond     *sync.Cond

	currentState    *safety.State
	currentWorkItem *source.WorkItem

	// Review configuration
	reviewConfig     review.Config
	skipReview       bool
	reviewOnly       bool
	reviewPassed     bool
	reviewRunner     *review.Runner
	pendingReviewFix bool   // true when Claude needs to fix review issues before next review
	lastReviewIssues string // formatted issues from last review for Claude to fix
}

// SetSource sets the source for the loop (for testing).
func (l *Loop) SetSource(src source.Source) {
	l.source = src
}

func New(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool) *Loop {
	return NewWithSource(config, workingDir, onOutput, onStateChange, streaming, nil)
}

func NewWithSource(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool, src source.Source) *Loop {
	l := &Loop{
		config:        config,
		workingDir:    workingDir,
		onOutput:      onOutput,
		onStateChange: onStateChange,
		streaming:     streaming,
		source:        src,
		reviewConfig:  review.ConfigFromEnv(),
	}
	l.pauseCond = sync.NewCond(&l.mu)
	return l
}

// SetReviewConfig sets the review configuration.
func (l *Loop) SetReviewConfig(cfg review.Config) {
	l.reviewConfig = cfg
}

// SetSkipReview disables the review phase.
func (l *Loop) SetSkipReview(skip bool) {
	l.skipReview = skip
}

// SetReviewOnly enables review-only mode (skips task phases).
func (l *Loop) SetReviewOnly(reviewOnly bool) {
	l.reviewOnly = reviewOnly
}

// loopAction indicates what the main loop should do next.
type loopAction int

const (
	loopContinue loopAction = iota
	loopReturn
	loopBreakToClaudeInvocation
)

// runContext holds mutable state for a single Run invocation.
type runContext struct {
	ctx                context.Context
	workItemID         string
	source             source.Source
	state              *safety.State
	result             *Result
	progressNotes      []string
	filesChangedSet    map[string]struct{}
	workItem           *source.WorkItem
	iterationSummaries []string // Track summaries for each iteration
}

// Deprecated: NewWithClient is deprecated. Use NewWithSource instead.
func NewWithClient(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool, _ any) *Loop {
	// This function exists for backwards compatibility during migration.
	// It will be removed in a future version.
	return NewWithSource(config, workingDir, onOutput, onStateChange, streaming, nil)
}

// checkStopRequested checks if stop was requested and handles the response.
// Returns loopReturn if we should exit, loopContinue otherwise.
func (l *Loop) checkStopRequested(rc *runContext) loopAction {
	l.mu.Lock()
	for l.paused && !l.stopRequested {
		l.pauseCond.Wait()
	}
	if l.stopRequested {
		l.mu.Unlock()
		l.log("Stop requested by user")
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: Stopped by user after %d iterations", rc.state.Iteration))
		rc.result.ExitReason = safety.ExitReasonUserInterrupt
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}
	l.mu.Unlock()
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

// handleAllPhasesComplete handles the logic when all phases are complete.
// Returns loopReturn if we should exit, loopBreakToClaudeInvocation if we should invoke Claude,
// or loopContinue to proceed normally.
func (l *Loop) handleAllPhasesComplete(rc *runContext) loopAction {
	if !rc.workItem.AllPhasesComplete() && !l.reviewOnly {
		return loopContinue
	}

	// If we have pending review fixes, invoke Claude to fix them
	if l.pendingReviewFix {
		l.log("Pending review fixes - invoking Claude to fix issues")
		return loopBreakToClaudeInvocation
	}

	// Check if we should run review
	if l.reviewConfig.Enabled && !l.skipReview && !l.reviewPassed {
		return l.handleReviewPhase(rc)
	}

	// No review needed or already passed
	return l.completeAllPhases(rc)
}

// handleReviewPhase handles the review phase when all tasks are complete.
func (l *Loop) handleReviewPhase(rc *runContext) loopAction {
	l.log("All phases complete - starting code review")
	reviewResult, reviewErr := l.runReview(rc.ctx, rc.workItemID, rc.source, rc.state, rc.result.TotalFilesChanged)
	if reviewErr != nil {
		l.log(fmt.Sprintf("Review error: %v", reviewErr))
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("error: Review failed: %v", reviewErr))
		rc.result.ExitReason = safety.ExitReasonError
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}

	if reviewResult.Passed {
		l.reviewPassed = true
		l.log("Review passed!")
		_ = rc.source.AddNote(rc.workItemID, "progress: Code review passed")
		return l.completeAllPhases(rc)
	}

	// Review found issues - log them and let Claude fix them
	l.log(fmt.Sprintf("Review found %d issues", reviewResult.TotalIssues))
	issueNote := review.FormatIssuesMarkdown(reviewResult.Results)
	_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("review: [iter %d] Review found %d issues:\n%s", rc.state.Iteration, reviewResult.TotalIssues, issueNote))

	// Check if we've exceeded max review iterations
	checkResult := safety.Check(l.config, rc.state)
	if checkResult.ShouldExit {
		l.log(fmt.Sprintf("Review safety exit: %s", checkResult.Reason))
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("error: Review safety exit: %s", checkResult.Reason))
		rc.result.ExitReason = checkResult.Reason
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}

	// Set pending fix flag so next iteration invokes Claude instead of re-running review
	l.pendingReviewFix = true
	l.lastReviewIssues = issueNote
	rc.progressNotes = append(rc.progressNotes, fmt.Sprintf("[iter %d] Review found %d issues - please fix them:\n%s", rc.state.Iteration, reviewResult.TotalIssues, issueNote))
	return loopBreakToClaudeInvocation
}

// completeAllPhases marks the work item as complete and returns.
func (l *Loop) completeAllPhases(rc *runContext) loopAction {
	l.log("All phases complete!")
	_ = rc.source.SetStatus(rc.workItemID, "closed")
	_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: Completed all phases in %d iterations", rc.state.Iteration))
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
	iterSummary := fmt.Sprintf("[iter %d] %s", rc.state.Iteration, status.Summary)
	if len(status.FilesChanged) > 0 {
		iterSummary += fmt.Sprintf(" (files: %s)", strings.Join(status.FilesChanged, ", "))
	} else {
		iterSummary += " (no files changed)"
	}
	rc.iterationSummaries = append(rc.iterationSummaries, iterSummary)

	rc.state.RecordIteration(status.FilesChanged, status.Error)

	// Reset pending review fix flag - Claude has attempted to fix the issues
	if l.pendingReviewFix {
		l.pendingReviewFix = false
		l.lastReviewIssues = ""
	}

	if l.onStateChange != nil {
		l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
	}

	if status.Status == parser.StatusDone {
		l.log("Claude reported DONE")
		_ = rc.source.SetStatus(rc.workItemID, "closed")
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: Completed in %d iterations", rc.state.Iteration))
		rc.result.ExitReason = safety.ExitReasonComplete
		rc.result.Iterations = rc.state.Iteration
		return loopReturn
	}

	if status.Status == parser.StatusBlocked {
		l.log(fmt.Sprintf("Claude reported BLOCKED: %s", status.Error))
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("error: [iter %d] BLOCKED: %s", rc.state.Iteration, status.Error))
		rc.result.ExitReason = safety.ExitReasonBlocked
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
		rc.progressNotes = append(rc.progressNotes, fmt.Sprintf("[iter %d] Completed: %s", rc.state.Iteration, status.PhaseCompleted))
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: [iter %d] Completed %s", rc.state.Iteration, status.PhaseCompleted))
	} else {
		rc.progressNotes = append(rc.progressNotes, fmt.Sprintf("[iter %d] %s", rc.state.Iteration, status.Summary))
		_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("progress: [iter %d] %s", rc.state.Iteration, status.Summary))
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
		src, workItemID = source.Detect(workItemID)
	}
	timing.Log("Loop.Run: source created")

	result := &Result{
		ExitReason:        safety.ExitReasonComplete,
		TotalFilesChanged: make([]string, 0),
	}
	defer func() { result.Duration = time.Since(startTime) }()

	timing.Log("Loop.Run: fetching work item")
	workItem, err := src.Get(workItemID)
	timing.Log("Loop.Run: work item fetched")
	if err != nil {
		result.ExitReason = safety.ExitReasonError
		return result, err
	}

	l.log(fmt.Sprintf("Starting on %s %s: %s", src.Type(), workItemID, workItem.Title))
	_ = src.SetStatus(workItemID, "in_progress")

	rc := &runContext{
		ctx:             ctx,
		workItemID:      workItemID,
		source:          src,
		state:           safety.NewState(),
		result:          result,
		progressNotes:   nil,
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
		// If action == loopBreakToClaudeInvocation, we fall through to invoke Claude

		if action != loopBreakToClaudeInvocation {
			rc.state.Iteration++

			checkResult := safety.Check(l.config, rc.state)
			if checkResult.ShouldExit {
				l.log(fmt.Sprintf("Safety exit: %s", checkResult.Reason))
				_ = rc.source.AddNote(rc.workItemID, fmt.Sprintf("error: Safety exit after %d iters: %s", rc.state.Iteration, checkResult.Reason))
				rc.result.ExitReason = checkResult.Reason
				rc.result.ExitMessage = checkResult.Message
				rc.result.Iterations = rc.state.Iteration
				rc.result.RecentSummaries = l.getRecentSummaries(rc, 5)
				return rc.result, nil
			}
		}

		currentPhase := rc.workItem.CurrentPhase()
		l.logIterationSeparator(rc.state.Iteration, l.config.MaxIterations)
		l.log(fmt.Sprintf("Iteration %d/%d", rc.state.Iteration, l.config.MaxIterations))
		if currentPhase != nil {
			l.log(fmt.Sprintf("Current phase: %s", currentPhase.Name))
		}

		promptText := prompt.Build(rc.workItem, rc.progressNotes)

		l.currentState = rc.state
		l.currentWorkItem = rc.workItem

		if l.onStateChange != nil {
			l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
		}

		l.log("Invoking Claude...")

		invoker := l.claudeInvoker
		if invoker == nil {
			invoker = l.invokeClaudePrint
		}
		output, err := invoker(ctx, promptText)
		if err != nil {
			rc.result.ExitReason = safety.ExitReasonError
			return rc.result, err
		}

		status, err := parser.Parse(output)
		if err != nil {
			rc.result.ExitReason = safety.ExitReasonError
			return rc.result, err
		}

		if status == nil {
			l.log("Warning: No PROGRAMMATOR_STATUS found in output")
			rc.state.RecordIteration(nil, "no_status_block")
			if l.onStateChange != nil {
				l.onStateChange(rc.state, rc.workItem, rc.result.TotalFilesChanged)
			}
			rc.progressNotes = append(rc.progressNotes, fmt.Sprintf("[iter %d] No status block returned", rc.state.Iteration))
			continue
		}

		if action := l.processClaudeStatus(rc, status); action == loopReturn {
			return rc.result, nil
		}
	}
}

func (l *Loop) invokeClaudePrint(ctx context.Context, promptText string) (string, error) {
	args := []string{"--print"}

	if l.config.ClaudeFlags != "" {
		args = append(args, strings.Fields(l.config.ClaudeFlags)...)
	}

	if l.streaming {
		args = append(args, "--output-format", "stream-json", "--verbose")
	}

	if l.permissionSocketPath != "" {
		hookSettings := l.buildHookSettings()
		args = append(args, "--settings", hookSettings)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(l.config.Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "claude", args...)
	if l.workingDir != "" {
		cmd.Dir = l.workingDir
	}

	// Explicitly pass through environment, with CLAUDE_CONFIG_DIR if configured
	cmd.Env = os.Environ()
	if l.config.ClaudeConfigDir != "" {
		cmd.Env = append(cmd.Env, "CLAUDE_CONFIG_DIR="+l.config.ClaudeConfigDir)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	stopStats := make(chan struct{})
	if l.onProcessStats != nil {
		go l.pollProcessStats(cmd.Process.Pid, stopStats)
	}

	go func() {
		defer stdin.Close()
		_, _ = io.WriteString(stdin, promptText)
	}()

	var output string
	if l.streaming {
		output = l.processStreamingOutput(stdout)
	} else {
		output = l.processTextOutput(stdout)
	}

	err = cmd.Wait()
	close(stopStats)
	if l.onProcessStats != nil {
		l.onProcessStats(0, 0) // Signal process ended
	}
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return timeoutBlockedStatus(), nil
		}
		return "", err
	}

	return output, nil
}

func (l *Loop) processTextOutput(stdout io.Reader) string {
	var output strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text() + "\n"
		output.WriteString(line)
		if l.onOutput != nil {
			l.onOutput(line)
		}
	}

	return output.String()
}

type messageUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u messageUsage) TotalInputTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

type modelUsageStats struct {
	InputTokens              int `json:"inputTokens"`
	OutputTokens             int `json:"outputTokens"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
	CacheReadInputTokens     int `json:"cacheReadInputTokens"`
}

func (u modelUsageStats) TotalInputTokens() int {
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	Model   string `json:"model"`
	Message struct {
		Model   string `json:"model"`
		Content []struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Name  string `json:"name,omitempty"`  // tool name
			Input any    `json:"input,omitempty"` // tool input
			ID    string `json:"id,omitempty"`    // tool use id
		} `json:"content"`
		Usage messageUsage `json:"usage"`
	} `json:"message"`
	ModelUsage map[string]modelUsageStats `json:"modelUsage"`
	Result     string                     `json:"result"`
	ToolName   string                     `json:"tool_name,omitempty"`   // for user events with tool results
	ToolResult string                     `json:"tool_result,omitempty"` // tool result content
}

func (l *Loop) processStreamingOutput(stdout io.Reader) string {
	var fullOutput strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			debug.Logf("stream: failed to parse JSON: %v (line: %.100s...)", err, line)
			continue
		}

		debug.Logf("stream: event type=%s subtype=%s", event.Type, event.Subtype)

		switch event.Type {
		case "system":
			l.handleSystemEvent(&event)
		case "assistant":
			l.handleAssistantEvent(&event, &fullOutput)
		case "user":
			debug.Logf("stream: user event (tool result?)")
		case "result":
			l.handleResultEvent(&event, &fullOutput)
		default:
			debug.Logf("stream: unhandled event type=%s", event.Type)
		}
	}

	return fullOutput.String()
}

func (l *Loop) handleSystemEvent(event *streamEvent) {
	if event.Subtype == "init" && event.Model != "" && l.currentState != nil {
		l.currentState.Model = event.Model
		l.notifyStateChange()
	}
}

func (l *Loop) handleAssistantEvent(event *streamEvent, fullOutput *strings.Builder) {
	if l.currentState != nil {
		l.currentState.SetCurrentIterTokens(
			event.Message.Usage.TotalInputTokens(),
			event.Message.Usage.OutputTokens,
		)
		l.notifyStateChange()
	}

	for _, block := range event.Message.Content {
		debug.Logf("stream: assistant block type=%s name=%s", block.Type, block.Name)
		l.handleContentBlock(&block, fullOutput)
	}
}

func (l *Loop) handleContentBlock(block *struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
	ID    string `json:"id,omitempty"`
}, fullOutput *strings.Builder) {
	if block.Type == "text" && block.Text != "" {
		fullOutput.WriteString(block.Text)
		if l.onOutput != nil {
			l.onOutput(block.Text)
		}
	} else if block.Type == "tool_use" && block.Name != "" {
		l.outputToolUse(block.Name, block.Input)
	}
}

func (l *Loop) outputToolUse(name string, input any) {
	if l.onOutput == nil {
		return
	}
	toolLine := name
	inputMap, hasInput := input.(map[string]any)
	if hasInput {
		toolLine += formatToolArg(name, inputMap)
	}
	l.onOutput(fmt.Sprintf("\n[TOOL]%s\n", toolLine))

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

	// Generate unified diff
	diff := udiff.Unified("old", "new", oldStr, newStr)
	if diff == "" {
		return
	}

	// Output each line with appropriate marker for TUI styling
	for line := range strings.SplitSeq(diff, "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			// Skip file headers
			continue
		case strings.HasPrefix(line, "@@"):
			l.onOutput(fmt.Sprintf("[DIFF@]%s\n", line))
		case strings.HasPrefix(line, "-"):
			l.onOutput(fmt.Sprintf("[DIFF-]%s\n", line))
		case strings.HasPrefix(line, "+"):
			l.onOutput(fmt.Sprintf("[DIFF+]%s\n", line))
		default:
			l.onOutput(fmt.Sprintf("[DIFF ]%s\n", line))
		}
	}
}

func (l *Loop) handleResultEvent(event *streamEvent, fullOutput *strings.Builder) {
	if l.currentState != nil && len(event.ModelUsage) > 0 {
		for model, usage := range event.ModelUsage {
			l.currentState.FinalizeIterTokens(
				model,
				usage.TotalInputTokens(),
				usage.OutputTokens,
			)
		}
		l.notifyStateChange()
	}
	if event.Result != "" && fullOutput.Len() == 0 {
		fullOutput.WriteString(event.Result)
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

func timeoutBlockedStatus() string {
	return `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Timeout"
  error: "Claude invocation timed out"`
}

func (l *Loop) Stop() {
	l.mu.Lock()
	l.stopRequested = true
	l.pauseCond.Broadcast()
	l.mu.Unlock()

	if l.cancelFunc != nil {
		l.cancelFunc()
	}
}

func (l *Loop) TogglePause() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.paused = !l.paused
	if !l.paused {
		l.pauseCond.Broadcast()
	}
	return l.paused
}

func (l *Loop) IsPaused() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.paused
}

func (l *Loop) log(message string) {
	if l.onOutput != nil {
		// [PROG] marker for TUI to detect and style with lipgloss
		l.onOutput(fmt.Sprintf("\n[PROG]%s\n", message))
	}
}

func (l *Loop) logIterationSeparator(iteration, maxIterations int) {
	if l.onOutput != nil {
		separator := fmt.Sprintf("\n\n---\n\n### 🔄 Iteration %d/%d\n\n", iteration, maxIterations)
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

func (l *Loop) SetClaudeInvoker(invoker ClaudeInvoker) {
	l.claudeInvoker = invoker
}

func (l *Loop) SetProcessStatsCallback(cb ProcessStatsCallback) {
	l.onProcessStats = cb
}

func (l *Loop) SetPermissionSocketPath(path string) {
	l.permissionSocketPath = path
}

func (l *Loop) buildHookSettings() string {
	hookCmd := fmt.Sprintf("programmator hook --socket %s", l.permissionSocketPath)

	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []map[string]any{
				{
					"matcher": "",
					"hooks": []map[string]any{
						{
							"type":    "command",
							"command": hookCmd,
							"timeout": 120000,
						},
					},
				},
			},
		},
	}

	data, _ := json.Marshal(settings)
	return string(data)
}

// runReview executes the review pipeline.
func (l *Loop) runReview(ctx context.Context, _ string, _ source.Source, state *safety.State, filesChanged []string) (*review.RunResult, error) {
	state.EnterReviewPhase()

	if l.reviewRunner == nil {
		// Adapt OutputCallback to review.OutputCallback
		var outputCallback review.OutputCallback
		if l.onOutput != nil {
			outputCallback = func(text string) {
				l.onOutput(text)
			}
		}
		l.reviewRunner = review.NewRunner(l.reviewConfig, outputCallback)
	}

	l.log(fmt.Sprintf("Running review iteration %d/%d", state.ReviewIterations+1, l.config.MaxReviewIterations))

	result, err := l.reviewRunner.Run(ctx, l.workingDir, filesChanged)

	// Record iteration AFTER review runs so the count reflects completed reviews
	state.RecordReviewIteration()

	if err != nil {
		state.ExitReviewPhase()
		return nil, err
	}

	if result.Passed {
		state.ExitReviewPhase()
	}

	return result, nil
}

// SetReviewRunner sets a custom review runner (useful for testing).
func (l *Loop) SetReviewRunner(runner *review.Runner) {
	l.reviewRunner = runner
}

// ReviewOnlyResult holds the result of a review-only run.
type ReviewOnlyResult struct {
	Passed        bool
	Iterations    int
	TotalIssues   int
	FilesFixed    []string
	Duration      time.Duration
	FinalReview   *review.RunResult
	ExitReason    safety.ExitReason
	CommitsMade   int
	LastReviewErr error
}

// RunReviewOnly runs the review-only loop: review → fix → commit → re-review.
// It requires git changed files to be provided and does not use tickets.
func (l *Loop) RunReviewOnly(baseBranch string, filesChanged []string) (*ReviewOnlyResult, error) {
	startTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFunc = cancel
	defer cancel()

	state := safety.NewState()
	result := &ReviewOnlyResult{
		Passed:     false,
		FilesFixed: make([]string, 0),
		ExitReason: safety.ExitReasonComplete,
	}
	defer func() { result.Duration = time.Since(startTime) }()

	// Force enable review
	l.reviewConfig.Enabled = true

	// Initialize review runner
	if l.reviewRunner == nil {
		var outputCallback review.OutputCallback
		if l.onOutput != nil {
			outputCallback = func(text string) {
				l.onOutput(text)
			}
		}
		l.reviewRunner = review.NewRunner(l.reviewConfig, outputCallback)
	}

	filesFixedSet := make(map[string]struct{})

	for {
		l.mu.Lock()
		for l.paused && !l.stopRequested {
			l.pauseCond.Wait()
		}
		if l.stopRequested {
			l.mu.Unlock()
			l.log("Stop requested by user")
			result.ExitReason = safety.ExitReasonUserInterrupt
			result.Iterations = state.Iteration
			return result, nil
		}
		l.mu.Unlock()

		select {
		case <-ctx.Done():
			result.ExitReason = safety.ExitReasonUserInterrupt
			result.Iterations = state.Iteration
			return result, nil
		default:
		}

		state.Iteration++
		result.Iterations = state.Iteration

		l.logIterationSeparator(state.Iteration, l.config.MaxIterations)
		l.log(fmt.Sprintf("Review iteration %d/%d", state.Iteration, l.config.MaxIterations))

		// Check safety limits
		checkResult := safety.Check(l.config, state)
		if checkResult.ShouldExit {
			l.log(fmt.Sprintf("Safety exit: %s", checkResult.Reason))
			result.ExitReason = checkResult.Reason
			return result, nil
		}

		// Run review
		l.log("Running code review...")
		reviewResult, err := l.reviewRunner.Run(ctx, l.workingDir, filesChanged)
		if err != nil {
			l.log(fmt.Sprintf("Review error: %v", err))
			result.LastReviewErr = err
			result.ExitReason = safety.ExitReasonError
			return result, err
		}

		result.FinalReview = reviewResult
		result.TotalIssues = reviewResult.TotalIssues

		if reviewResult.Passed {
			l.log("Review passed - no issues found!")
			result.Passed = true
			result.ExitReason = safety.ExitReasonComplete
			return result, nil
		}

		l.log(fmt.Sprintf("Review found %d issues - invoking Claude to fix", reviewResult.TotalIssues))

		// Build prompt for Claude to fix issues
		issuesMarkdown := review.FormatIssuesMarkdown(reviewResult.Results)
		promptText := BuildReviewFixPrompt(baseBranch, filesChanged, issuesMarkdown, state.Iteration)

		l.currentState = state
		if l.onStateChange != nil {
			l.onStateChange(state, nil, result.FilesFixed)
		}

		// Invoke Claude to fix issues
		l.log("Invoking Claude to fix review issues...")
		invoker := l.claudeInvoker
		if invoker == nil {
			invoker = l.invokeClaudePrint
		}
		output, err := invoker(ctx, promptText)
		if err != nil {
			result.ExitReason = safety.ExitReasonError
			return result, err
		}

		// Parse status from Claude's response
		status, err := parser.Parse(output)
		if err != nil {
			l.log(fmt.Sprintf("Warning: Failed to parse status: %v", err))
			state.RecordIteration(nil, "parse_error")
			continue
		}

		if status == nil {
			l.log("Warning: No PROGRAMMATOR_STATUS found in output")
			state.RecordIteration(nil, "no_status_block")
			continue
		}

		l.log(fmt.Sprintf("Status: %s", status.Status))
		l.log(fmt.Sprintf("Summary: %s", status.Summary))

		// Track files changed
		if len(status.FilesChanged) > 0 {
			l.log(fmt.Sprintf("Files fixed: %s", strings.Join(status.FilesChanged, ", ")))
			for _, f := range status.FilesChanged {
				if _, exists := filesFixedSet[f]; !exists {
					filesFixedSet[f] = struct{}{}
					result.FilesFixed = append(result.FilesFixed, f)
				}
			}
		}

		state.RecordIteration(status.FilesChanged, status.Error)

		if l.onStateChange != nil {
			l.onStateChange(state, nil, result.FilesFixed)
		}

		// Handle blocked status
		if status.Status == parser.StatusBlocked {
			l.log(fmt.Sprintf("Claude reported BLOCKED: %s", status.Error))
			result.ExitReason = safety.ExitReasonBlocked
			return result, nil
		}

		// Handle commits: if Claude made changes but didn't commit, we auto-commit
		if len(status.FilesChanged) > 0 {
			if status.CommitMade {
				result.CommitsMade++
				l.log(fmt.Sprintf("Commit made by Claude (total: %d)", result.CommitsMade))
			} else {
				// Auto-commit changes since Claude didn't
				l.log("Auto-committing changes...")
				if err := l.autoCommitChanges(status.FilesChanged, status.Summary); err != nil {
					l.log(fmt.Sprintf("Warning: auto-commit failed: %v", err))
				} else {
					result.CommitsMade++
					l.log(fmt.Sprintf("Auto-commit successful (total: %d)", result.CommitsMade))
				}
			}
		}

		// Refresh the list of changed files for next review iteration
		refreshedFiles, err := getChangedFilesForReview(l.workingDir, baseBranch)
		if err != nil {
			l.log(fmt.Sprintf("Warning: failed to refresh changed files: %v", err))
		} else {
			filesChanged = refreshedFiles
		}

		// If Claude reports DONE, check if review passes
		if status.Status == parser.StatusDone {
			l.log("Claude reports fixes complete - running final review")
			// Loop will continue and run review again
		}
	}
}

// autoCommitChanges stages and commits the specified files with a fix message.
func (l *Loop) autoCommitChanges(files []string, summary string) error {
	// Stage files
	for _, file := range files {
		cmd := exec.Command("git", "-C", l.workingDir, "add", file)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stage %s: %w", file, err)
		}
	}

	// Create commit message
	commitMsg := "fix: review fixes"
	if summary != "" {
		commitMsg = fmt.Sprintf("fix: %s", summary)
	}

	// Commit
	cmd := exec.Command("git", "-C", l.workingDir, "commit", "-m", commitMsg)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("commit failed: %w: %s", err, string(output))
	}

	return nil
}

// getChangedFilesForReview returns the list of files changed between base branch and HEAD.
func getChangedFilesForReview(workingDir, baseBranch string) ([]string, error) {
	// Try three-dot diff first (changes since branching)
	cmd := exec.Command("git", "-C", workingDir, "diff", "--name-only", baseBranch+"...HEAD")
	out, err := cmd.Output()
	if err != nil {
		// Fallback to two-dot diff
		cmd = exec.Command("git", "-C", workingDir, "diff", "--name-only", baseBranch)
		out, err = cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git diff failed: %w", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}

// BuildReviewFixPrompt creates a prompt for Claude to fix review issues.
func BuildReviewFixPrompt(baseBranch string, filesChanged []string, issuesMarkdown string, iteration int) string {
	filesList := strings.Join(filesChanged, "\n  - ")

	return fmt.Sprintf(`You are reviewing and fixing code issues found by automated code review.

## Context
- Base branch: %s
- Review iteration: %d

## Files to review
  - %s

## Issues Found
The following issues were found by code review agents and need to be fixed:

%s

## Instructions
1. Review each issue carefully
2. Make the necessary fixes to address each issue
3. After fixing, commit your changes with a clear commit message
4. Report your status

## Important
- Fix ALL issues listed above
- Make clean, minimal fixes that address the specific issues
- Test your changes if possible
- Commit with message format: "fix: <brief description of fixes>"

## Session End Protocol
When you've completed your fixes, you MUST end with exactly this block:

`+"```"+`
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - file1.go
    - file2.go
  summary: "Fixed N issues: brief description"
  commit_made: true
`+"```"+`

Status values:
- CONTINUE: Made fixes, ready for re-review
- DONE: All issues fixed, commit made
- BLOCKED: Cannot fix without human intervention (add error: field)

If blocked:
`+"```"+`
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "What was attempted"
  error: "Description of what's blocking progress"
`+"```"+`
`, baseBranch, iteration, filesList, issuesMarkdown)
}
