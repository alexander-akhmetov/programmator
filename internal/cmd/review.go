package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/worksonmyai/programmator/internal/config"
	"github.com/worksonmyai/programmator/internal/git"
	"github.com/worksonmyai/programmator/internal/loop"
	"github.com/worksonmyai/programmator/internal/progress"
	"github.com/worksonmyai/programmator/internal/prompt"
	"github.com/worksonmyai/programmator/internal/review"
)

var (
	reviewBaseBranch      string
	reviewWorkingDir      string
	reviewSkipPermissions bool
	reviewGuardMode       bool
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Run code review on git diff without a ticket",
	Long: `Run code review on the current git diff without requiring a ticket.

By default, reviews changes from main branch to HEAD (main...HEAD).
Use --base to specify a different base branch.

Examples:
  programmator review                    # Review changes vs main
  programmator review --base=develop     # Review changes vs develop
  programmator review -d /path/to/repo   # Review specific directory`,
	RunE: runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewBaseBranch, "base", "main", "Base branch to diff against (default: main)")
	reviewCmd.Flags().StringVarP(&reviewWorkingDir, "dir", "d", "", "Working directory (default: current directory)")
	reviewCmd.Flags().BoolVar(&reviewSkipPermissions, "dangerously-skip-permissions", false, "Skip interactive permission dialogs (grants all permissions)")
	reviewCmd.Flags().BoolVar(&reviewGuardMode, "guard", true, "Guard mode: skip permissions but block destructive commands via dcg (default: enabled)")
}

func runReview(_ *cobra.Command, _ []string) error {
	wd := reviewWorkingDir
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Verify we're in a git repo
	if !isGitRepo(wd) {
		return fmt.Errorf("not a git repository: %s", wd)
	}

	// Get list of changed files
	filesChanged, err := git.ChangedFiles(wd, reviewBaseBranch)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}

	if len(filesChanged) == 0 {
		fmt.Println("No changes to review.")
		return nil
	}

	fmt.Printf("Reviewing %d changed files (diff %s...HEAD):\n", len(filesChanged), reviewBaseBranch)
	for _, f := range filesChanged {
		fmt.Printf("  • %s\n", f)
	}
	fmt.Println()

	// Load unified config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	safetyConfig := cfg.ToSafetyConfig()

	if reviewGuardMode {
		if _, err := exec.LookPath("dcg"); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: dcg not found, falling back to interactive permissions. Install: https://github.com/Dicklesworthstone/destructive_command_guard")
			reviewGuardMode = false
		} else {
			if safetyConfig.ClaudeFlags == "" {
				safetyConfig.ClaudeFlags = "--dangerously-skip-permissions"
			} else if !strings.Contains(safetyConfig.ClaudeFlags, "--dangerously-skip-permissions") {
				safetyConfig.ClaudeFlags += " --dangerously-skip-permissions"
			}
		}
	}

	if reviewSkipPermissions {
		if safetyConfig.ClaudeFlags == "" {
			safetyConfig.ClaudeFlags = "--dangerously-skip-permissions"
		} else if !strings.Contains(safetyConfig.ClaudeFlags, "--dangerously-skip-permissions") {
			safetyConfig.ClaudeFlags += " --dangerously-skip-permissions"
		}
	}

	reviewConfig := cfg.ToReviewConfig()
	// Create progress logger for review
	// Use base branch + directory name as source ID
	sourceID := fmt.Sprintf("review-%s-%s", reviewBaseBranch, filepath.Base(wd))
	progressLogger, err := progress.NewLogger(progress.Config{
		LogsDir:    cfg.LogsDir,
		SourceID:   sourceID,
		SourceType: "review",
		WorkDir:    wd,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create progress logger: %v\n", err)
	}
	closeLogger := func() {
		if progressLogger != nil {
			progressLogger.Close()
		}
	}

	// Create loop for review-only mode
	reviewLoop := loop.New(safetyConfig, wd, func(text string) {
		fmt.Print(text)
	}, nil, true)
	reviewLoop.SetReviewConfig(reviewConfig)
	reviewLoop.SetReviewOnly(true)
	reviewLoop.SetGuardMode(reviewGuardMode)
	if progressLogger != nil {
		reviewLoop.SetProgressLogger(progressLogger)
	}
	promptBuilder, err := prompt.NewBuilder(cfg.Prompts)
	if err != nil {
		return fmt.Errorf("failed to create prompt builder: %w", err)
	}
	reviewLoop.SetPromptBuilder(promptBuilder)

	// Run review-only loop
	result, err := reviewLoop.RunReviewOnly(reviewBaseBranch, filesChanged)
	if err != nil {
		closeLogger()
		return fmt.Errorf("review failed: %w", err)
	}

	// Print summary
	printReviewOnlySummary(result)

	closeLogger()
	if !result.Passed {
		os.Exit(1) // Non-zero exit for CI integration
	}

	return nil
}

// isGitRepo checks if the directory is a git repository.
func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// Styles for summary output
var (
	reviewSummaryBorder = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(lipgloss.Color("205")).
				Padding(0, 2)

	reviewSummaryTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("205")).
				Align(lipgloss.Center)

	reviewSummaryLabel = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	reviewSummaryValue = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	reviewSummarySuccess = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true)

	reviewSummaryError = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")).
				Bold(true)
)

func formatReviewDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int64(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := int64(d / time.Second)

	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func printReviewOnlySummary(result *loop.ReviewOnlyResult) {
	var b strings.Builder

	b.WriteString(reviewSummaryTitle.Render("REVIEW COMPLETE") + "\n\n")

	if result.Passed {
		b.WriteString(reviewSummaryLabel.Render("Status:     ") + reviewSummarySuccess.Render("PASSED") + "\n")
	} else {
		b.WriteString(reviewSummaryLabel.Render("Status:     ") + reviewSummaryError.Render("FAILED") + "\n")
	}

	b.WriteString(reviewSummaryLabel.Render("Iterations: ") + reviewSummaryValue.Render(fmt.Sprintf("%d", result.Iterations)) + "\n")
	b.WriteString(reviewSummaryLabel.Render("Issues:     ") + reviewSummaryValue.Render(fmt.Sprintf("%d", result.TotalIssues)) + "\n")
	b.WriteString(reviewSummaryLabel.Render("Duration:   ") + reviewSummaryValue.Render(formatReviewDuration(result.Duration)) + "\n")

	if result.CommitsMade > 0 {
		b.WriteString(reviewSummaryLabel.Render("Commits:    ") + reviewSummaryValue.Render(fmt.Sprintf("%d", result.CommitsMade)) + "\n")
	}

	if len(result.FilesFixed) > 0 {
		b.WriteString(reviewSummaryLabel.Render("Files fixed:") + "\n")
		for _, f := range result.FilesFixed {
			b.WriteString("  • " + f + "\n")
		}
	}

	if !result.Passed && result.FinalReview != nil && len(result.FinalReview.Results) > 0 {
		b.WriteString("\n" + reviewSummaryLabel.Render("Remaining issues:") + "\n")
		b.WriteString(review.FormatIssuesMarkdown(result.FinalReview.Results))
	}

	fmt.Println()
	fmt.Println(reviewSummaryBorder.Render(b.String()))
}
