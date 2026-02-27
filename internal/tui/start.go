package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/dirs"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
	"github.com/alexander-akhmetov/programmator/internal/timing"
)

var (
	workingDir      string
	maxIterations   int
	stagnationLimit int
	timeout         int
	reviewOnly      bool

	// Git workflow flags
	autoCommit         bool
	moveCompletedPlans bool
	autoBranch         bool
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
	startCmd.Flags().BoolVar(&reviewOnly, "review-only", false, "Run only the code review phase (skip task phases)")

	// Git workflow flags
	startCmd.Flags().BoolVar(&autoCommit, "auto-commit", false, "Auto-commit changes after each phase completion")
	startCmd.Flags().BoolVar(&moveCompletedPlans, "move-completed", false, "Move completed plan files to plans/completed/")
	startCmd.Flags().BoolVar(&autoBranch, "branch", false, "Create a new branch (programmator/<source>) before starting")
}

func runStart(_ *cobra.Command, args []string) error {
	timing.Start()
	timing.Log("runStart: begin")
	ticketID := args[0]

	timing.Log("runStart: loading config")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Apply CLI flag overrides
	cfg.ApplyCLIFlags(maxIterations, stagnationLimit, timeout)

	// Convert configs
	safetyConfig := cfg.ToSafetyConfig()
	execConfig := cfg.ToExecutorConfig()

	wd, err := resolveWorkingDir(workingDir)
	if err != nil {
		return err
	}

	if err := writeSessionFile(ticketID, wd); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write session file: %v\n", err)
	}
	defer removeSessionFile()
	timing.Log("runStart: session file written")

	timing.Log("runStart: creating TUI")
	t := New(safetyConfig)
	t.SetReviewOnly(reviewOnly)
	t.SetReviewConfig(cfg.ToReviewConfig())
	promptBuilder, err := prompt.NewBuilder(cfg.Prompts)
	if err != nil {
		return fmt.Errorf("failed to create prompt builder: %w", err)
	}
	t.SetPromptBuilder(promptBuilder)
	t.SetTicketCommand(cfg.TicketCommand)
	t.SetHideTips(cfg.HideTips)

	// Set git workflow config from CLI flags and config file
	t.SetGitWorkflowConfig(loop.GitWorkflowConfig{
		AutoCommit:         autoCommit || cfg.Git.AutoCommit,
		MoveCompletedPlans: moveCompletedPlans || cfg.Git.MoveCompletedPlans,
		CompletedPlansDir:  cfg.Git.CompletedPlansDir,
		BranchPrefix:       cfg.Git.BranchPrefix,
		AutoBranch:         autoBranch,
	})

	// Wire executor config
	t.SetExecutorConfig(execConfig)

	timing.Log("TUI created, calling Run")
	_, err = t.Run(ticketID, wd)
	timing.Log("runStart: TUI.Run returned")

	if err != nil {
		return fmt.Errorf("loop error: %w", err)
	}

	return nil
}

func sessionFilePath() string {
	return filepath.Join(dirs.StateDir(), "session.json")
}

func writeSessionFile(ticketID, workingDir string) error {
	path := sessionFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.Marshal(map[string]any{
		"ticket_id":   ticketID,
		"working_dir": workingDir,
		"started_at":  time.Now().Format(time.RFC3339),
		"pid":         os.Getpid(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

func removeSessionFile() {
	os.Remove(sessionFilePath())
}
