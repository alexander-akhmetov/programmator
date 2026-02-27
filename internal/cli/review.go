package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/git"
	"github.com/alexander-akhmetov/programmator/internal/review"
)

var errReviewFailed = fmt.Errorf("review failed: issues found")

var (
	reviewBaseBranch string
	reviewWorkDir    string
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
	SilenceErrors: true,
	RunE:          runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewBaseBranch, "base", "main", "Base branch to diff against (default: main)")
	reviewCmd.Flags().StringVarP(&reviewWorkDir, "dir", "d", "", "Working directory (default: current directory)")
}

func runReview(_ *cobra.Command, _ []string) error {
	wd, err := resolveWorkingDir(reviewWorkDir)
	if err != nil {
		return err
	}

	if !git.IsInsideRepo(wd) {
		return fmt.Errorf("not a git repository: %s", wd)
	}

	filesChanged, err := git.ChangedFiles(wd, reviewBaseBranch)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}

	if len(filesChanged) == 0 {
		fmt.Println("No changes to review.")
		return nil
	}

	fmt.Printf("Reviewing %d changed files (vs %s):\n", len(filesChanged), reviewBaseBranch)
	for _, f := range filesChanged {
		fmt.Printf("  %s\n", f)
	}
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	reviewConfig, err := cfg.ToReviewConfig()
	if err != nil {
		return fmt.Errorf("invalid review config: %w", err)
	}

	runner := review.NewRunner(reviewConfig)

	result, err := runner.RunIteration(context.Background(), wd, filesChanged)
	if err != nil {
		return fmt.Errorf("review failed: %w", err)
	}

	printReviewSummary(result)

	if !result.Passed {
		return errReviewFailed
	}

	return nil
}

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

func printReviewSummary(result *review.RunResult) {
	tty := stdoutIsTTY()
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(maybeBold(tty, "REVIEW COMPLETE") + "\n\n")

	if result.Passed {
		b.WriteString(maybeDim(tty, "Status:     ") + maybeFgBold(tty, colorGreen, "PASSED") + "\n")
	} else {
		b.WriteString(maybeDim(tty, "Status:     ") + maybeFgBold(tty, colorRed, "FAILED") + "\n")
	}

	b.WriteString(maybeDim(tty, "Iterations: ") + fmt.Sprintf("%d", result.Iteration) + "\n")
	b.WriteString(maybeDim(tty, "Issues:     ") + fmt.Sprintf("%d", result.TotalIssues) + "\n")
	b.WriteString(maybeDim(tty, "Duration:   ") + formatReviewDuration(result.Duration) + "\n")

	if !result.Passed && len(result.Results) > 0 {
		b.WriteString("\n" + maybeDim(tty, "Remaining issues:") + "\n")
		b.WriteString(review.FormatIssuesMarkdown(result.Results))
	}

	fmt.Println(b.String())
}
