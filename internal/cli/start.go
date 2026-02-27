package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
)

var (
	startWorkingDir      string
	startMaxIterations   int
	startStagnationLimit int
	startTimeout         int

	// Git workflow flags
	startAutoCommit         bool
	startMoveCompletedPlans bool
	startAutoBranch         bool
)

var startCmd = &cobra.Command{
	Use:   "start <ticket-id>",
	Short: "Start loop on ticket",
	Long: `Start the programmator loop on a ticket or plan file.

The loop will:
1. Read the ticket/plan and identify the current phase
2. Invoke the configured coding agent with a structured prompt
3. Parse the response for status updates
4. Loop until all phases are complete or safety limits are reached

Events are streamed to stdout with a sticky progress footer in TTY mode.
In non-TTY mode (pipes, CI), output is plain text without ANSI escapes.`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	startCmd.Flags().StringVarP(&startWorkingDir, "dir", "d", "", "Working directory (default: current directory)")
	startCmd.Flags().IntVarP(&startMaxIterations, "max-iterations", "n", 0, "Maximum iterations")
	startCmd.Flags().IntVar(&startStagnationLimit, "stagnation-limit", 0, "Stagnation limit")
	startCmd.Flags().IntVar(&startTimeout, "timeout", 0, "Timeout per Claude invocation in seconds")

	startCmd.Flags().BoolVar(&startAutoCommit, "auto-commit", false, "Auto-commit changes after each phase completion")
	startCmd.Flags().BoolVar(&startMoveCompletedPlans, "move-completed", false, "Move completed plan files to plans/completed/")
	startCmd.Flags().BoolVar(&startAutoBranch, "branch", false, "Create a new branch (programmator/<source>) before starting")
}

func runStart(_ *cobra.Command, args []string) error {
	sourceID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	cfg.ApplyCLIFlags(startMaxIterations, startStagnationLimit, startTimeout)

	wd, err := resolveWorkingDir(startWorkingDir)
	if err != nil {
		return err
	}

	promptBuilder, err := prompt.NewBuilder(cfg.Prompts)
	if err != nil {
		return fmt.Errorf("failed to create prompt builder: %w", err)
	}

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	termWidth, termHeight := 0, 0
	if isTTY {
		termWidth, termHeight, _ = term.GetSize(int(os.Stdout.Fd()))
	}

	runCfg := RunConfig{
		SafetyConfig:  cfg.ToSafetyConfig(),
		PromptBuilder: promptBuilder,
		TicketCommand: cfg.TicketCommand,
		GitWorkflowConfig: loop.GitWorkflowConfig{
			AutoCommit:         startAutoCommit || cfg.Git.AutoCommit,
			MoveCompletedPlans: startMoveCompletedPlans || cfg.Git.MoveCompletedPlans,
			CompletedPlansDir:  cfg.Git.CompletedPlansDir,
			BranchPrefix:       cfg.Git.BranchPrefix,
			AutoBranch:         startAutoBranch,
		},
		ExecutorConfig: cfg.ToExecutorConfig(),
		IsTTY:          isTTY,
		TermWidth:      termWidth,
		TermHeight:     termHeight,
	}

	reviewCfg, err := cfg.ToReviewConfig()
	if err != nil {
		return fmt.Errorf("invalid review config: %w", err)
	}
	runCfg.ReviewConfig = reviewCfg

	_, err = Run(context.Background(), sourceID, wd, runCfg)
	if err != nil {
		return fmt.Errorf("loop error: %w", err)
	}

	return nil
}
