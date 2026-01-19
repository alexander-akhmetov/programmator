// Package loop implements the main orchestration loop.
package loop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/ticket"
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
type ClaudeInvoker func(ctx context.Context, promptText string) (string, error)

type Loop struct {
	config        safety.Config
	workingDir    string
	onOutput      OutputCallback
	onStateChange StateCallback
	streaming     bool
	cancelFunc    context.CancelFunc
	client        ticket.Client
	claudeInvoker ClaudeInvoker

	mu            sync.Mutex
	paused        bool
	stopRequested bool
	pauseCond     *sync.Cond
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
	}
	l.pauseCond = sync.NewCond(&l.mu)
	return l
}

func (l *Loop) Run(ticketID string) (*Result, error) {
	startTime := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFunc = cancel
	defer cancel()

	client := l.client
	if client == nil {
		client = ticket.NewClient()
	}
	state := safety.NewState()

	result := &Result{
		ExitReason:        safety.ExitReasonComplete,
		TotalFilesChanged: make([]string, 0),
	}
	defer func() { result.Duration = time.Since(startTime) }()

	t, err := client.Get(ticketID)
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

		if t.AllPhasesComplete() {
			l.log("All phases complete!")
			_ = client.SetStatus(ticketID, "closed")
			_ = client.AddNote(ticketID, fmt.Sprintf("progress: Completed all phases in %d iterations", state.Iteration))
			result.ExitReason = safety.ExitReasonComplete
			result.Iterations = state.Iteration
			return result, nil
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
			_ = client.UpdatePhase(ticketID, status.PhaseCompleted)
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

	if err := cmd.Wait(); err != nil {
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

type streamEvent struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
	Result string `json:"result"`
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
		case "assistant":
			for _, block := range event.Message.Content {
				if block.Type == "text" && block.Text != "" {
					fullOutput.WriteString(block.Text)
					if l.onOutput != nil {
						l.onOutput(block.Text)
					}
				}
			}
		case "result":
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
		l.onOutput(fmt.Sprintf("**▶ programmator:** %s\n", message))
	}
}

func (l *Loop) logIterationSeparator(iteration, maxIterations int) {
	if l.onOutput != nil {
		separator := fmt.Sprintf("\n---\n\n### 🔄 Iteration %d/%d\n\n", iteration, maxIterations)
		l.onOutput(separator)
	}
}

func (r *Result) FilesChangedList() []string {
	return r.TotalFilesChanged
}

func (l *Loop) SetClaudeInvoker(invoker ClaudeInvoker) {
	l.claudeInvoker = invoker
}
