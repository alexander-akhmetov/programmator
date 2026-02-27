package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// RunConfig holds all configuration needed to run the loop.
type RunConfig struct {
	SafetyConfig      safety.Config
	ReviewConfig      review.Config
	PromptBuilder     *prompt.Builder
	TicketCommand     string
	GitWorkflowConfig loop.GitWorkflowConfig
	ExecutorConfig    llm.ExecutorConfig
	Out               io.Writer // output writer (default: os.Stdout)
	IsTTY             bool
	TermWidth         int
}

// Run creates a loop, wires callbacks to a Writer, and runs synchronously.
// It handles signal-based shutdown and guarantees footer cleanup on exit.
func Run(ctx context.Context, sourceID, workingDir string, cfg RunConfig) (*loop.Result, error) {
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}

	w := NewWriter(out, cfg.IsTTY, cfg.TermWidth)

	l := loop.New(
		cfg.SafetyConfig,
		workingDir,
		func(state *safety.State, workItem *domain.WorkItem, _ []string) {
			w.UpdateFooter(state, workItem, cfg.SafetyConfig)
		},
		true,
	)

	l.SetEventCallback(func(ev event.Event) {
		w.WriteEvent(ev)
	})
	l.SetProcessStatsCallback(func(pid int, memoryKB int64) {
		w.SetProcessStats(pid, memoryKB)
	})

	l.SetReviewConfig(cfg.ReviewConfig)
	if cfg.PromptBuilder != nil {
		l.SetPromptBuilder(cfg.PromptBuilder)
	}
	if cfg.TicketCommand != "" {
		l.SetTicketCommand(cfg.TicketCommand)
	}
	l.SetGitWorkflowConfig(cfg.GitWorkflowConfig)
	l.SetExecutorConfig(cfg.ExecutorConfig)

	// Signal handling — stop loop on SIGINT/SIGTERM.
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run loop synchronously in the main goroutine.
	// The loop uses its own context internally, but we stop it on signal.
	go func() {
		<-ctx.Done()
		l.Stop()
	}()

	result, err := l.Run(sourceID)

	// Always clean up the footer before returning.
	w.ClearFooter()

	if err != nil {
		return result, err
	}

	// Print final summary.
	printRunSummary(w, result)

	return result, nil
}

// printRunSummary prints a compact summary after the loop finishes.
func printRunSummary(w *Writer, result *loop.Result) {
	if result == nil {
		return
	}

	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, w.style(colorDim, "────────────────────────────"))

	status := w.styleBold(colorGreen, string(result.ExitReason))
	if result.ExitReason == safety.ExitReasonBlocked ||
		result.ExitReason == safety.ExitReasonError ||
		result.ExitReason == safety.ExitReasonReviewFailed {
		status = w.styleBold(colorRed, string(result.ExitReason))
	}

	fmt.Fprintf(w.out, "%s %s", w.style(colorDim, "Exit:"), status)
	if result.ExitMessage != "" {
		fmt.Fprintf(w.out, " %s", w.style(colorDim, "("+result.ExitMessage+")"))
	}
	fmt.Fprintln(w.out)

	fmt.Fprintf(w.out, "%s %s  %s %s  %s %s\n",
		w.style(colorDim, "Iterations:"), w.style(colorWhite, fmt.Sprintf("%d", result.Iterations)),
		w.style(colorDim, "Files:"), w.style(colorWhite, fmt.Sprintf("%d", len(result.TotalFilesChanged))),
		w.style(colorDim, "Duration:"), w.style(colorWhite, formatDuration(result.Duration)),
	)
}
