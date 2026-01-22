// Package loop implements the main orchestration loop.
package loop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/ticket"
	"github.com/alexander-akhmetov/programmator/internal/timing"
)

type Result struct {
	ExitReason        safety.ExitReason
	Iterations        int
	TotalFilesChanged []string
	FinalStatus       *parser.ParsedStatus
	Duration          time.Duration
}

type OutputCallback func(text string)
type StateCallback func(state *safety.State, ticket *ticket.Ticket, filesChanged []string)
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
	client               ticket.Client
	claudeInvoker        ClaudeInvoker
	permissionSocketPath string

	mu            sync.Mutex
	paused        bool
	stopRequested bool
	pauseCond     *sync.Cond

	currentState  *safety.State
	currentTicket *ticket.Ticket

	// Review configuration
	reviewConfig      review.Config
	skipReview        bool
	reviewOnly        bool
	reviewPassed      bool
	reviewRunner      *review.Runner
	pendingReviewFix  bool   // true when Claude needs to fix review issues before next review
	lastReviewIssues  string // formatted issues from last review for Claude to fix
}

func New(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool) *Loop {
	return NewWithClient(config, workingDir, onOutput, onStateChange, streaming, nil)
}

func NewWithClient(config safety.Config, workingDir string, onOutput OutputCallback, onStateChange StateCallback, streaming bool, client ticket.Client) *Loop {
	l := &Loop{
		config:        config,
		workingDir:    workingDir,
		onOutput:      onOutput,
		onStateChange: onStateChange,
		streaming:     streaming,
		client:        client,
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

func (l *Loop) Run(ticketID string) (*Result, error) {
	timing.Log("Loop.Run: start")
	startTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFunc = cancel
	defer cancel()

	timing.Log("Loop.Run: creating ticket client")
	client := l.client
	if client == nil {
		client = ticket.NewClient()
	}
	timing.Log("Loop.Run: ticket client created")
	state := safety.NewState()

	result := &Result{
		ExitReason:        safety.ExitReasonComplete,
		TotalFilesChanged: make([]string, 0),
	}
	defer func() { result.Duration = time.Since(startTime) }()

	timing.Log("Loop.Run: fetching ticket")
	t, err := client.Get(ticketID)
	timing.Log("Loop.Run: ticket fetched")
	if err != nil {
		result.ExitReason = safety.ExitReasonError
		return result, err
	}

	l.log(fmt.Sprintf("Starting on ticket %s: %s", ticketID, t.Title))
	_ = client.SetStatus(ticketID, "in_progress")

	var progressNotes []string
	filesChangedSet := make(map[string]struct{})

	if l.onStateChange != nil {
		l.onStateChange(state, t, nil)
	}

	for {
		l.mu.Lock()
		for l.paused && !l.stopRequested {
			l.pauseCond.Wait()
		}
		if l.stopRequested {
			l.mu.Unlock()
			l.log("Stop requested by user")
			_ = client.AddNote(ticketID, fmt.Sprintf("progress: Stopped by user after %d iterations", state.Iteration))
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

		t, err = client.Get(ticketID)
		if err != nil {
			result.ExitReason = safety.ExitReasonError
			return result, err
		}

		if t.AllPhasesComplete() || l.reviewOnly {
			// If we have pending review fixes, skip review and let Claude fix them first
			if l.pendingReviewFix {
				l.log("Pending review fixes - invoking Claude to fix issues")
				// Fall through to Claude invocation below
			} else if l.reviewConfig.Enabled && !l.skipReview && !l.reviewPassed {
				// Check if we should run review
				l.log("All phases complete - starting code review")
				reviewResult, reviewErr := l.runReview(ctx, ticketID, client, state, result.TotalFilesChanged)
				if reviewErr != nil {
					l.log(fmt.Sprintf("Review error: %v", reviewErr))
					_ = client.AddNote(ticketID, fmt.Sprintf("error: Review failed: %v", reviewErr))
					result.ExitReason = safety.ExitReasonError
					result.Iterations = state.Iteration
					return result, reviewErr
				}

				if !reviewResult.Passed {
					// Review found issues - log them and let Claude fix them
					l.log(fmt.Sprintf("Review found %d issues", reviewResult.TotalIssues))
					issueNote := review.FormatIssuesMarkdown(reviewResult.Results)
					_ = client.AddNote(ticketID, fmt.Sprintf("review: [iter %d] Review found %d issues:\n%s", state.Iteration, reviewResult.TotalIssues, issueNote))

					// Check if we've exceeded max review iterations
					checkResult := safety.Check(l.config, state)
					if checkResult.ShouldExit {
						l.log(fmt.Sprintf("Review safety exit: %s", checkResult.Reason))
						_ = client.AddNote(ticketID, fmt.Sprintf("error: Review safety exit: %s", checkResult.Reason))
						result.ExitReason = checkResult.Reason
						result.Iterations = state.Iteration
						return result, nil
					}

					// Set pending fix flag so next iteration invokes Claude instead of re-running review
					l.pendingReviewFix = true
					l.lastReviewIssues = issueNote
					progressNotes = append(progressNotes, fmt.Sprintf("[iter %d] Review found %d issues - please fix them:\n%s", state.Iteration, reviewResult.TotalIssues, issueNote))
					// Fall through to Claude invocation below
				} else {
					l.reviewPassed = true
					l.log("Review passed!")
					_ = client.AddNote(ticketID, "progress: Code review passed")

					l.log("All phases complete!")
					_ = client.SetStatus(ticketID, "closed")
					_ = client.AddNote(ticketID, fmt.Sprintf("progress: Completed all phases in %d iterations", state.Iteration))
					result.ExitReason = safety.ExitReasonComplete
					result.Iterations = state.Iteration
					return result, nil
				}
			} else {
				// No review needed or already passed
				l.log("All phases complete!")
				_ = client.SetStatus(ticketID, "closed")
				_ = client.AddNote(ticketID, fmt.Sprintf("progress: Completed all phases in %d iterations", state.Iteration))
				result.ExitReason = safety.ExitReasonComplete
				result.Iterations = state.Iteration
				return result, nil
			}
		}

		state.Iteration++

		checkResult := safety.Check(l.config, state)
		if checkResult.ShouldExit {
			l.log(fmt.Sprintf("Safety exit: %s", checkResult.Reason))
			_ = client.AddNote(ticketID, fmt.Sprintf("error: Safety exit after %d iters: %s", state.Iteration, checkResult.Reason))
			result.ExitReason = checkResult.Reason
			result.Iterations = state.Iteration
			return result, nil
		}

		currentPhase := t.CurrentPhase()
		l.logIterationSeparator(state.Iteration, l.config.MaxIterations)
		l.log(fmt.Sprintf("Iteration %d/%d", state.Iteration, l.config.MaxIterations))
		if currentPhase != nil {
			l.log(fmt.Sprintf("Current phase: %s", currentPhase.Name))
		}

		promptText := prompt.Build(t, progressNotes)

		l.currentState = state
		l.currentTicket = t

		if l.onStateChange != nil {
			l.onStateChange(state, t, result.TotalFilesChanged)
		}

		l.log("Invoking Claude...")

		invoker := l.claudeInvoker
		if invoker == nil {
			invoker = l.invokeClaudePrint
		}
		output, err := invoker(ctx, promptText)
		if err != nil {
			result.ExitReason = safety.ExitReasonError
			return result, err
		}

		status, err := parser.Parse(output)
		if err != nil {
			result.ExitReason = safety.ExitReasonError
			return result, err
		}

		if status == nil {
			l.log("Warning: No PROGRAMMATOR_STATUS found in output")
			state.RecordIteration(nil, "no_status_block")
			if l.onStateChange != nil {
				l.onStateChange(state, t, result.TotalFilesChanged)
			}
			progressNotes = append(progressNotes, fmt.Sprintf("[iter %d] No status block returned", state.Iteration))
			continue
		}

		l.log(fmt.Sprintf("Status: %s", status.Status))
		l.log(fmt.Sprintf("Summary: %s", status.Summary))

		result.FinalStatus = status

		if status.PhaseCompleted != "" {
			l.log(fmt.Sprintf("Phase completed: %s", status.PhaseCompleted))
			if err := client.UpdatePhase(ticketID, status.PhaseCompleted); err != nil {
				l.log(fmt.Sprintf("Warning: failed to update phase '%s': %v", status.PhaseCompleted, err))
			}
			progressNotes = append(progressNotes, fmt.Sprintf("[iter %d] Completed: %s", state.Iteration, status.PhaseCompleted))
			_ = client.AddNote(ticketID, fmt.Sprintf("progress: [iter %d] Completed %s", state.Iteration, status.PhaseCompleted))
		} else {
			progressNotes = append(progressNotes, fmt.Sprintf("[iter %d] %s", state.Iteration, status.Summary))
			_ = client.AddNote(ticketID, fmt.Sprintf("progress: [iter %d] %s", state.Iteration, status.Summary))
		}

		if len(status.FilesChanged) > 0 {
			l.log(fmt.Sprintf("Files changed: %s", strings.Join(status.FilesChanged, ", ")))
			for _, f := range status.FilesChanged {
				if _, exists := filesChangedSet[f]; !exists {
					filesChangedSet[f] = struct{}{}
					result.TotalFilesChanged = append(result.TotalFilesChanged, f)
				}
			}
		}

		state.RecordIteration(status.FilesChanged, status.Error)

		// Reset pending review fix flag - Claude has attempted to fix the issues
		if l.pendingReviewFix {
			l.pendingReviewFix = false
			l.lastReviewIssues = ""
		}

		if l.onStateChange != nil {
			l.onStateChange(state, t, result.TotalFilesChanged)
		}

		if status.Status == parser.StatusDone {
			l.log("Claude reported DONE")
			_ = client.SetStatus(ticketID, "closed")
			_ = client.AddNote(ticketID, fmt.Sprintf("progress: Completed in %d iterations", state.Iteration))
			result.ExitReason = safety.ExitReasonComplete
			result.Iterations = state.Iteration
			return result, nil
		}

		if status.Status == parser.StatusBlocked {
			l.log(fmt.Sprintf("Claude reported BLOCKED: %s", status.Error))
			_ = client.AddNote(ticketID, fmt.Sprintf("error: [iter %d] BLOCKED: %s", state.Iteration, status.Error))
			result.ExitReason = safety.ExitReasonBlocked
			result.Iterations = state.Iteration
			return result, nil
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
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage messageUsage `json:"usage"`
	} `json:"message"`
	ModelUsage map[string]modelUsageStats `json:"modelUsage"`
	Result     string                     `json:"result"`
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
			continue
		}

		switch event.Type {
		case "system":
			if event.Subtype == "init" && event.Model != "" && l.currentState != nil {
				l.currentState.Model = event.Model
				if l.onStateChange != nil && l.currentTicket != nil {
					l.onStateChange(l.currentState, l.currentTicket, nil)
				}
			}
		case "assistant":
			if l.currentState != nil {
				l.currentState.SetCurrentIterTokens(
					event.Message.Usage.TotalInputTokens(),
					event.Message.Usage.OutputTokens,
				)
				if l.onStateChange != nil && l.currentTicket != nil {
					l.onStateChange(l.currentState, l.currentTicket, nil)
				}
			}
			for _, block := range event.Message.Content {
				if block.Type == "text" && block.Text != "" {
					fullOutput.WriteString(block.Text)
					if l.onOutput != nil {
						l.onOutput(block.Text)
					}
				}
			}
		case "result":
			if l.currentState != nil && len(event.ModelUsage) > 0 {
				for model, usage := range event.ModelUsage {
					l.currentState.FinalizeIterTokens(
						model,
						usage.TotalInputTokens(),
						usage.OutputTokens,
					)
				}
				if l.onStateChange != nil && l.currentTicket != nil {
					l.onStateChange(l.currentState, l.currentTicket, nil)
				}
			}
			if event.Result != "" && fullOutput.Len() == 0 {
				fullOutput.WriteString(event.Result)
			}
		}
	}

	return fullOutput.String()
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
func (l *Loop) runReview(ctx context.Context, _ string, _ ticket.Client, state *safety.State, filesChanged []string) (*review.RunResult, error) {
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
