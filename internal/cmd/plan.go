package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/worksonmyai/programmator/internal/config"
	"github.com/worksonmyai/programmator/internal/input"
	"github.com/worksonmyai/programmator/internal/parser"
	"github.com/worksonmyai/programmator/internal/progress"
	"github.com/worksonmyai/programmator/internal/prompt"
)

var (
	planOutputDir string
	planMaxTurns  int
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Plan management commands",
	Long:  `Commands for creating and managing implementation plans.`,
}

var planCreateCmd = &cobra.Command{
	Use:   "create <description>",
	Short: "Create a new plan interactively",
	Long: `Create a new implementation plan by interacting with Claude.

Claude will analyze your codebase and ask clarifying questions to understand
what you want to build. Once it has enough information, it generates a
structured plan file that can be executed with 'programmator start'.

Examples:
  programmator plan create "Add user authentication with JWT"
  programmator plan create "Refactor the database layer to use connection pooling"
  programmator plan create "Add comprehensive error handling to the API"`,
	Args: cobra.ExactArgs(1),
	RunE: runPlanCreate,
}

func init() {
	planCreateCmd.Flags().StringVarP(&planOutputDir, "output", "o", "", "Directory to save the plan (default: ./plans)")
	planCreateCmd.Flags().IntVar(&planMaxTurns, "max-turns", 10, "Maximum Q&A turns before generating plan")

	planCmd.AddCommand(planCreateCmd)
}

func runPlanCreate(_ *cobra.Command, args []string) error {
	description := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Set up output directory
	outDir := planOutputDir
	if outDir == "" {
		outDir = filepath.Join(wd, "plans")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create progress logger for the plan creation session
	progressLogger, err := progress.NewLogger(progress.Config{
		LogsDir:    cfg.LogsDir,
		SourceID:   "plan-create",
		SourceType: "plan-create",
		WorkDir:    wd,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create progress logger: %v\n", err)
	} else {
		defer progressLogger.Close()
	}

	// Create prompt builder
	prompts, err := config.LoadPrompts(cfg.ConfigDir(), "")
	if err != nil {
		return fmt.Errorf("failed to load prompts: %w", err)
	}
	builder, err := prompt.NewBuilder(prompts)
	if err != nil {
		return fmt.Errorf("failed to create prompt builder: %w", err)
	}

	// Create input collector
	collector := input.NewTerminalCollector()

	// Run the plan creation loop
	creator := &planCreator{
		description:    description,
		workDir:        wd,
		outDir:         outDir,
		maxTurns:       planMaxTurns,
		builder:        builder,
		collector:      collector,
		progressLogger: progressLogger,
		claudeFlags:    cfg.ClaudeFlags,
	}

	planPath, err := creator.run()
	if err != nil {
		return err
	}

	// Print success message
	fmt.Println()
	fmt.Println(successStyle.Render("✓ Plan created successfully!"))
	fmt.Println(fileStyle.Render("  " + planPath))
	fmt.Println()
	fmt.Println(hintStyle.Render("Run with: programmator start " + planPath))

	return nil
}

var (
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)
	fileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))
	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
	questionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)
	contextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)
	turnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

type planCreator struct {
	description    string
	workDir        string
	outDir         string
	maxTurns       int
	builder        *prompt.Builder
	collector      input.Collector
	progressLogger *progress.Logger
	claudeFlags    string
	qa             []prompt.QA
}

func (p *planCreator) run() (string, error) {
	ctx := context.Background()

	for turn := 1; turn <= p.maxTurns; turn++ {
		fmt.Println(turnStyle.Render(fmt.Sprintf("\n--- Turn %d/%d ---", turn, p.maxTurns)))

		// Build prompt
		promptText, err := p.builder.BuildPlanCreate(p.description, p.qa)
		if err != nil {
			return "", fmt.Errorf("build prompt: %w", err)
		}

		p.logProgressf("Turn %d: invoking Claude", turn)

		// Invoke Claude
		output, err := p.invokeClaude(ctx, promptText)
		if err != nil {
			return "", fmt.Errorf("invoke Claude: %w", err)
		}

		// Check for plan ready signal
		if parser.HasPlanReadySignal(output) {
			planContent, err := parser.ParsePlanContent(output)
			if err != nil {
				return "", fmt.Errorf("parse plan content: %w", err)
			}

			p.logProgressf("Plan ready, saving to file")
			return p.savePlan(planContent)
		}

		// Check for question signal
		if parser.HasQuestionSignal(output) {
			question, err := parser.ParseQuestionPayload(output)
			if err != nil {
				return "", fmt.Errorf("parse question: %w", err)
			}

			// Display question
			fmt.Println()
			if question.Context != "" {
				fmt.Println(contextStyle.Render(question.Context))
			}
			fmt.Println(questionStyle.Render(question.Question))

			// Collect answer
			answer, err := p.collector.AskQuestion(ctx, question.Question, question.Options)
			if err != nil {
				return "", fmt.Errorf("collect answer: %w", err)
			}

			p.logProgressf("Turn %d: Q=%s A=%s", turn, question.Question, answer)

			// Record Q&A
			p.qa = append(p.qa, prompt.QA{
				Question: question.Question,
				Answer:   answer,
			})

			continue
		}

		// No signal found - Claude might have generated a plan without the signal
		// or there was an issue. Let's check if the output looks like a plan
		if looksLikePlan(output) {
			p.logProgressf("Plan appears complete (no signal), saving")
			return p.savePlan(output)
		}

		// If we reach here, Claude didn't produce a question or plan
		// This might happen if Claude needs more context - continue to next turn
		p.logProgressf("Turn %d: no signal found, continuing", turn)
		fmt.Println(hintStyle.Render("Claude is analyzing the codebase..."))
	}

	return "", fmt.Errorf("max turns (%d) reached without generating a plan", p.maxTurns)
}

func (p *planCreator) invokeClaude(ctx context.Context, promptText string) (string, error) {
	args := []string{"--print"}

	if p.claudeFlags != "" {
		args = append(args, strings.Fields(p.claudeFlags)...)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "claude", args...)
	cmd.Dir = p.workDir
	cmd.Env = os.Environ()

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

	// Read output
	var output strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		output.WriteString(line + "\n")
		// Print progress dots for long operations
		if strings.Contains(line, "Reading") || strings.Contains(line, "Searching") {
			fmt.Print(".")
		}
	}

	if err := cmd.Wait(); err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude invocation timed out")
		}
		return "", err
	}

	return output.String(), nil
}

func (p *planCreator) savePlan(content string) (string, error) {
	// Generate filename: <date>-<slug>.md
	slug := generateSlug(p.description)
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", date, slug)
	planPath := filepath.Join(p.outDir, filename)

	// Ensure content has proper plan structure
	if !strings.HasPrefix(content, "# ") {
		// Add a title if missing
		content = fmt.Sprintf("# Plan: %s\n\n%s", p.description, content)
	}

	if err := os.WriteFile(planPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write plan file: %w", err)
	}

	return planPath, nil
}

func (p *planCreator) logProgressf(format string, args ...any) {
	if p.progressLogger != nil {
		p.progressLogger.Printf(format, args...)
	}
}

// generateSlug creates a URL-safe slug from a description.
func generateSlug(description string) string {
	// Lowercase
	slug := strings.ToLower(description)

	// Replace spaces and special chars with hyphens
	var result strings.Builder
	prevHyphen := false
	for _, r := range slug {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			result.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			result.WriteRune('-')
			prevHyphen = true
		}
	}

	// Trim hyphens and limit length
	slug = strings.Trim(result.String(), "-")
	if len(slug) > 50 {
		slug = slug[:50]
		// Don't end with a hyphen
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}

// looksLikePlan checks if output appears to be a valid plan (has tasks).
func looksLikePlan(output string) bool {
	// Check for plan indicators
	hasTitle := strings.Contains(output, "# Plan:") || strings.Contains(output, "# Plan ")
	hasTasks := strings.Contains(output, "- [ ]") || strings.Contains(output, "## Tasks")
	return hasTitle && hasTasks
}
