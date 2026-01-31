package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/worksonmyai/programmator/internal/config"
	"github.com/worksonmyai/programmator/internal/loop"
	"github.com/worksonmyai/programmator/internal/progress"
	"github.com/worksonmyai/programmator/internal/prompt"
	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/source"
	"github.com/worksonmyai/programmator/internal/timing"
	"github.com/worksonmyai/programmator/internal/tui"
)

var (
	workingDir      string
	maxIterations   int
	stagnationLimit int
	timeout         int
	skipPermissions bool
	guardMode       bool
	allowPatterns   []string
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
	startCmd.Flags().BoolVar(&guardMode, "guard", true, "Guard mode: skip permissions but block destructive commands via dcg (default: enabled)")
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

	// Apply CLI flag overrides
	cfg.ApplyCLIFlags(maxIterations, stagnationLimit, timeout)

	// Convert to legacy safety.Config for compatibility
	safetyConfig := cfg.ToSafetyConfig()

	wd, err := resolveWorkingDir(workingDir)
	if err != nil {
		return err
	}

	if err := writeSessionFile(ticketID, wd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write session file: %v\n", err)
	}
	defer removeSessionFile()
	timing.Log("runStart: session file written")

	guardMode = resolveGuardMode(guardMode, &safetyConfig)
	if skipPermissions {
		applySkipPermissions(&safetyConfig)
	}

	// Create progress logger
	sourceType := protocol.SourceTypeTicket
	if source.IsPlanPath(ticketID) {
		sourceType = protocol.SourceTypePlan
	}
	progressLogger, err := progress.NewLogger(progress.Config{
		LogsDir:    cfg.LogsDir,
		SourceID:   ticketID,
		SourceType: sourceType,
		WorkDir:    wd,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create progress logger: %v\n", err)
	} else {
		defer progressLogger.Close()
	}

	timing.Log("runStart: creating TUI")
	t := tui.New(safetyConfig)
	t.SetInteractivePermissions(!skipPermissions && !guardMode)
	t.SetGuardMode(guardMode)
	t.SetAllowPatterns(allowPatterns)
	t.SetReviewOnly(reviewOnly)
	t.SetReviewConfig(cfg.ToReviewConfig())
	promptBuilder, err := prompt.NewBuilder(cfg.Prompts)
	if err != nil {
		return fmt.Errorf("failed to create prompt builder: %w", err)
	}
	t.SetPromptBuilder(promptBuilder)
	if progressLogger != nil {
		t.SetProgressLogger(progressLogger)
	}
	t.SetTicketCommand(cfg.TicketCommand)

	// Set git workflow config from CLI flags and config file
	gitConfig := loop.GitWorkflowConfig{
		AutoCommit:         autoCommit || cfg.Git.AutoCommit,
		MoveCompletedPlans: moveCompletedPlans || cfg.Git.MoveCompletedPlans,
		CompletedPlansDir:  cfg.Git.CompletedPlansDir,
		BranchPrefix:       cfg.Git.BranchPrefix,
		AutoBranch:         autoBranch,
	}
	if gitConfig.AutoCommit || gitConfig.MoveCompletedPlans || gitConfig.AutoBranch {
		t.SetGitWorkflowConfig(gitConfig)
	}

	timing.Log("TUI created, calling Run")
	_, err = t.Run(ticketID, wd)
	timing.Log("runStart: TUI.Run returned")

	if err != nil {
		return fmt.Errorf("loop error: %w", err)
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
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	content := fmt.Sprintf(`{"ticket_id": %q, "working_dir": %q, "started_at": %q, "pid": %d}`,
		ticketID, workingDir, time.Now().Format(time.RFC3339), os.Getpid())

	return os.WriteFile(path, []byte(content), 0600)
}

func removeSessionFile() {
	os.Remove(sessionFilePath())
}
