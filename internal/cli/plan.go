package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/parser"
	"github.com/alexander-akhmetov/programmator/internal/prompt"
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
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	outDir := planOutputDir
	if outDir == "" {
		outDir = filepath.Join(wd, "plans")
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	prompts, err := config.LoadPrompts(cfg.ConfigDir(), "")
	if err != nil {
		return fmt.Errorf("failed to load prompts: %w", err)
	}
	builder, err := prompt.NewBuilder(prompts)
	if err != nil {
		return fmt.Errorf("failed to create prompt builder: %w", err)
	}

	collector := NewTerminalCollector()

	execConfig := cfg.ToExecutorConfig()
	execConfig.Name = cfg.PlanExecutorOrDefault()
	creator := &planCreator{
		description:    description,
		workDir:        wd,
		outDir:         outDir,
		maxTurns:       planMaxTurns,
		builder:        builder,
		collector:      collector,
		executorConfig: execConfig,
	}

	planPath, err := creator.run()
	if err != nil {
		return err
	}

	tty := stdoutIsTTY()
	fmt.Println()
	fmt.Println(maybeFgBold(tty, colorGreen, "Plan created successfully!"))
	fmt.Println(maybeFg(tty, colorCyan, "  "+planPath))
	fmt.Println()
	fmt.Println(maybeDim(tty, "Run with: programmator start "+planPath))

	return nil
}

type planCreator struct {
	description    string
	workDir        string
	outDir         string
	maxTurns       int
	builder        *prompt.Builder
	collector      Collector
	executorConfig llm.ExecutorConfig
	qa             []prompt.QA
}

func (p *planCreator) run() (string, error) {
	ctx := context.Background()

	tty := stdoutIsTTY()
	for turn := 1; turn <= p.maxTurns; turn++ {
		fmt.Println(maybeDim(tty, fmt.Sprintf("\n--- Turn %d/%d ---", turn, p.maxTurns)))

		promptText, err := p.builder.BuildPlanCreate(p.description, p.qa)
		if err != nil {
			return "", fmt.Errorf("build prompt: %w", err)
		}

		output, err := p.invokeClaude(ctx, promptText)
		if err != nil {
			return "", fmt.Errorf("invoke executor: %w", err)
		}

		if parser.HasPlanReadySignal(output) {
			planContent, err := parser.ParsePlanContent(output)
			if err != nil {
				return "", fmt.Errorf("parse plan content: %w", err)
			}

			return p.savePlan(planContent)
		}

		if parser.HasQuestionSignal(output) {
			question, err := parser.ParseQuestionPayload(output)
			if err != nil {
				return "", fmt.Errorf("parse question: %w", err)
			}

			fmt.Println()
			if question.Context != "" {
				fmt.Println(maybeDim(tty, question.Context))
			}
			fmt.Println(maybeFgBold(tty, colorMagenta, question.Question))

			answer, err := p.collector.AskQuestion(ctx, question.Question, question.Options)
			if err != nil {
				return "", fmt.Errorf("collect answer: %w", err)
			}

			p.qa = append(p.qa, prompt.QA{
				Question: question.Question,
				Answer:   answer,
			})

			continue
		}

		if looksLikePlan(output) {
			return p.savePlan(output)
		}

		fmt.Println(maybeDim(tty, "Claude is analyzing the codebase..."))
	}

	return "", fmt.Errorf("max turns (%d) reached without generating a plan", p.maxTurns)
}

func (p *planCreator) invokeClaude(ctx context.Context, promptText string) (string, error) {
	inv, err := llm.NewInvoker(p.executorConfig)
	if err != nil {
		return "", fmt.Errorf("create invoker: %w", err)
	}

	opts := llm.InvokeOptions{
		WorkingDir: p.workDir,
		ExtraFlags: p.executorConfig.ExtraFlags,
		Timeout:    int((5 * time.Minute).Seconds()),
		OnOutput: func(text string) {
			if strings.Contains(text, "Reading") || strings.Contains(text, "Searching") {
				fmt.Print(".")
			}
		},
	}

	res, err := inv.Invoke(ctx, promptText, opts)
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

func (p *planCreator) savePlan(content string) (string, error) {
	slug := generateSlug(p.description)
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", date, slug)
	planPath := filepath.Join(p.outDir, filename)

	if !strings.HasPrefix(content, "# ") {
		content = fmt.Sprintf("# Plan: %s\n\n%s", p.description, content)
	}

	if err := os.WriteFile(planPath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("write plan file: %w", err)
	}

	return planPath, nil
}

// generateSlug creates a URL-safe slug from a description.
func generateSlug(description string) string {
	slug := strings.ToLower(description)

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

	slug = strings.Trim(result.String(), "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}

// looksLikePlan checks if output appears to be a valid plan (has tasks).
func looksLikePlan(output string) bool {
	hasTitle := strings.Contains(output, "# Plan:") || strings.Contains(output, "# Plan ")
	hasTasks := strings.Contains(output, "- [ ]") || strings.Contains(output, "## Tasks")
	return hasTitle && hasTasks
}
