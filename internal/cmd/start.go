package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/timing"
	"github.com/alexander-akhmetov/programmator/internal/tui"
)

var (
	workingDir      string
	maxIterations   int
	stagnationLimit int
	timeout         int
	skipPermissions bool
	allowPatterns   []string
	skipReview      bool
	reviewOnly      bool
)

var startCmd = &cobra.Command{
	Use:   "start <ticket-id>",
	Short: "Start loop on ticket",
	Long: `Start the programmator loop on a ticket.

The loop will:
1. Read the ticket and identify the current phase
2. Invoke Claude Code with a structured prompt
3. Parse the response for status updates
4. Loop until all phases are complete or safety limits are reached

The TUI displays real-time status including:
- Current iteration and stagnation counters
- Current phase being worked on
- Live log output from Claude

Controls:
  p - Pause/resume the loop
  s - Stop the loop
  q - Quit (stops loop if running)
  ↑/↓ - Scroll logs`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	startCmd.Flags().StringVarP(&workingDir, "dir", "d", "", "Working directory for Claude (default: current directory)")
	startCmd.Flags().IntVarP(&maxIterations, "max-iterations", "n", 0, "Maximum iterations (overrides PROGRAMMATOR_MAX_ITERATIONS)")
	startCmd.Flags().IntVar(&stagnationLimit, "stagnation-limit", 0, "Stagnation limit (overrides PROGRAMMATOR_STAGNATION_LIMIT)")
	startCmd.Flags().IntVar(&timeout, "timeout", 0, "Timeout per Claude invocation in seconds (overrides PROGRAMMATOR_TIMEOUT)")
	startCmd.Flags().BoolVar(&skipPermissions, "dangerously-skip-permissions", false, "Skip interactive permission dialogs (grants all permissions)")
	startCmd.Flags().StringArrayVar(&allowPatterns, "allow", nil, "Pre-allow permission patterns (e.g., 'Bash(git:*)', 'Read')")
	startCmd.Flags().BoolVar(&skipReview, "skip-review", false, "Skip the code review phase after all task phases complete")
	startCmd.Flags().BoolVar(&reviewOnly, "review-only", false, "Run only the code review phase (skip task phases)")
}

func runStart(_ *cobra.Command, args []string) error {
	timing.Start()
	timing.Log("runStart: begin")
	ticketID := args[0]

	timing.Log("runStart: loading config")
	config := safety.ConfigFromEnv()
	if maxIterations > 0 {
		config.MaxIterations = maxIterations
	}
	if stagnationLimit > 0 {
		config.StagnationLimit = stagnationLimit
	}
	if timeout > 0 {
		config.Timeout = timeout
	}

	wd := workingDir
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	if err := writeSessionFile(ticketID, wd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write session file: %v\n", err)
	}
	defer removeSessionFile()
	timing.Log("runStart: session file written")

	if skipPermissions {
		if config.ClaudeFlags == "" {
			config.ClaudeFlags = "--dangerously-skip-permissions"
		} else {
			config.ClaudeFlags += " --dangerously-skip-permissions"
		}
	}

	timing.Log("runStart: creating TUI")
	t := tui.New(config)
	t.SetInteractivePermissions(!skipPermissions)
	t.SetAllowPatterns(allowPatterns)
	t.SetSkipReview(skipReview)
	t.SetReviewOnly(reviewOnly)
	timing.Log("TUI created, calling Run")
	result, err := t.Run(ticketID, wd)
	timing.Log("runStart: TUI.Run returned")

	if err != nil {
		return fmt.Errorf("loop error: %w", err)
	}

	if result != nil {
		printSummary(ticketID, result)
	}

	return nil
}

func sessionFilePath() string {
	dir := os.Getenv("PROGRAMMATOR_STATE_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".programmator")
	}
	return filepath.Join(dir, "session.json")
}

func writeSessionFile(ticketID, workingDir string) error {
	path := sessionFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	content := fmt.Sprintf(`{"ticket_id": %q, "working_dir": %q, "started_at": %q, "pid": %d}`,
		ticketID, workingDir, time.Now().Format(time.RFC3339), os.Getpid())

	return os.WriteFile(path, []byte(content), 0644)
}

func removeSessionFile() {
	os.Remove(sessionFilePath())
}

var (
	summaryBorder = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("205")).
			Padding(0, 2)

	summaryTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Align(lipgloss.Center)

	summaryLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	summaryValue = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	summarySuccess = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	summaryWarning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208"))

	summaryError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	summaryFile = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))
)

func printSummary(ticketID string, result *loop.Result) {
	var b strings.Builder

	b.WriteString(summaryTitle.Render("PROGRAMMATOR COMPLETE") + "\n\n")
	b.WriteString(summaryLabel.Render("Ticket:     ") + summaryValue.Render(ticketID) + "\n")

	exitStyle := summaryValue
	switch result.ExitReason {
	case safety.ExitReasonComplete:
		exitStyle = summarySuccess
	case safety.ExitReasonBlocked, safety.ExitReasonError, safety.ExitReasonReviewFailed:
		exitStyle = summaryError
	case safety.ExitReasonMaxIterations, safety.ExitReasonStagnation, safety.ExitReasonUserInterrupt, safety.ExitReasonMaxReviewRetries:
		exitStyle = summaryWarning
	}
	b.WriteString(summaryLabel.Render("Exit:       ") + exitStyle.Render(string(result.ExitReason)) + "\n")

	// Show exit message if available (explains why exit happened)
	if result.ExitMessage != "" {
		b.WriteString(summaryLabel.Render("Reason:     ") + summaryWarning.Render(result.ExitMessage) + "\n")
	}

	b.WriteString(summaryLabel.Render("Iterations: ") + summaryValue.Render(fmt.Sprintf("%d", result.Iterations)) + "\n")
	b.WriteString(summaryLabel.Render("Duration:   ") + summaryValue.Render(formatDuration(result.Duration)) + "\n")

	if result.FinalStatus != nil && result.FinalStatus.Summary != "" {
		b.WriteString(summaryLabel.Render("Summary:    ") + summaryValue.Render(result.FinalStatus.Summary) + "\n")
	}

	if len(result.TotalFilesChanged) > 0 {
		b.WriteString("\n" + summaryLabel.Render(fmt.Sprintf("Files changed (%d):", len(result.TotalFilesChanged))) + "\n")
		for _, f := range result.TotalFilesChanged {
			b.WriteString("  " + summaryFile.Render("• "+f) + "\n")
		}
	}

	// Show recent iteration summaries for stagnation/max_iterations debugging
	if len(result.RecentSummaries) > 0 &&
		(result.ExitReason == safety.ExitReasonStagnation || result.ExitReason == safety.ExitReasonMaxIterations) {
		b.WriteString("\n" + summaryLabel.Render("Recent iterations:") + "\n")
		for _, summary := range result.RecentSummaries {
			b.WriteString("  " + summaryValue.Render(summary) + "\n")
		}
	}

	fmt.Println()
	fmt.Println(summaryBorder.Render(b.String()))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
