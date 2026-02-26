package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/permission"
)

var (
	runWorkingDir      string
	runSkipPermissions bool
	runAllowPatterns   []string
	runNonInteractive  bool
	runMaxTurns        int
	runExecutor        string
)

var runCmd = &cobra.Command{
	Use:   "run [prompt]",
	Short: "Run Claude with a custom prompt",
	Long: `Run Claude Code with a custom prompt (no ticket required).

The prompt can be provided as an argument or piped via stdin.

Examples:
  programmator run "explain this codebase"
  programmator run "fix the bug in main.go"
  echo "add tests for the parser" | programmator run
  programmator run --max-turns 5 "refactor the auth module"`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().StringVarP(&runWorkingDir, "dir", "d", "", "Working directory for Claude (default: current directory)")
	runCmd.Flags().BoolVar(&runSkipPermissions, "dangerously-skip-permissions", false, "Skip interactive permission dialogs (grants all permissions)")
	runCmd.Flags().StringArrayVar(&runAllowPatterns, "allow", nil, "Pre-allow permission patterns (e.g., 'Bash(git:*)', 'Read')")
	runCmd.Flags().BoolVar(&runNonInteractive, "print", false, "Non-interactive mode: print output directly without TUI")
	runCmd.Flags().IntVar(&runMaxTurns, "max-turns", 0, "Maximum agentic turns (0 = unlimited)")
	runCmd.Flags().StringVar(&runExecutor, "executor", "", "Executor to use (default: claude)")
}

// buildPrompt assembles the prompt from CLI args or stdin.
// Returns the prompt string or an error if no prompt is available.
func buildPrompt(args []string, stdin io.Reader) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}

	if f, ok := stdin.(*os.File); ok {
		stat, err := f.Stat()
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(f)
			if err != nil {
				return "", fmt.Errorf("failed to read from stdin: %w", err)
			}
			if p := strings.TrimSpace(string(data)); p != "" {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("no prompt provided. Usage: programmator run \"your prompt here\"")
}

func runRun(_ *cobra.Command, args []string) error {
	prompt, err := buildPrompt(args, os.Stdin)
	if err != nil {
		return err
	}

	wd, err := resolveWorkingDir(runWorkingDir)
	if err != nil {
		return err
	}

	if runNonInteractive {
		return runClaudePrint(prompt, wd)
	}

	return runClaudeTUI(prompt, wd)
}

// buildCommonFlags returns CLI flags shared by both print and TUI modes.
func buildCommonFlags() []string {
	var flags []string
	if runSkipPermissions {
		flags = append(flags, "--dangerously-skip-permissions")
	}
	if runMaxTurns > 0 {
		flags = append(flags, "--max-turns", fmt.Sprintf("%d", runMaxTurns))
	}
	return flags
}

func runClaudePrint(prompt, workingDir string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	execCfg := cfg.ToExecutorConfig()
	if runExecutor != "" {
		execCfg.Name = runExecutor
	}

	inv, err := llm.NewInvoker(execCfg)
	if err != nil {
		return fmt.Errorf("create invoker: %w", err)
	}

	opts := llm.InvokeOptions{
		WorkingDir: workingDir,
		ExtraFlags: append(execCfg.ExtraFlags, buildCommonFlags()...),
		OnOutput: func(text string) {
			fmt.Print(text)
		},
	}

	_, err = inv.Invoke(context.Background(), prompt, opts)
	return err
}

// runClaudeTUI runs Claude in interactive (non-print) mode with stdout/stderr
// pipes for TUI display. This intentionally uses exec.Command directly because
// it is not a --print invocation — it runs an interactive Claude session.
// For non-claude executors, falls back to print mode since interactive TUI is Claude-specific.
func runClaudeTUI(prompt, workingDir string) error {
	executorName := runExecutor
	if executorName == "" {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		executorName = cfg.Executor
	}
	if executorName != "" && executorName != "claude" {
		return runClaudePrint(prompt, workingDir)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var permServer *permission.Server
	if !runSkipPermissions {
		var err error
		permServer, err = permission.NewServer(workingDir, func(_ *permission.Request) permission.HandlerResponse {
			// For now, in run mode we auto-deny unknown permissions
			// TODO: Add interactive permission dialog for run mode
			return permission.HandlerResponse{Decision: permission.DecisionDeny}
		})
		if err != nil {
			return fmt.Errorf("failed to start permission server: %w", err)
		}
		defer permServer.Close()

		if len(runAllowPatterns) > 0 {
			permServer.SetPreAllowed(runAllowPatterns)
		}

		go func() {
			if err := permServer.Serve(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: permission server error: %v\n", err)
			}
		}()
	}

	// Build Claude command
	args := buildCommonFlags()

	if !runSkipPermissions && permServer != nil {
		hookSettings := llm.BuildHookSettings(llm.HookConfig{
			PermissionSocketPath: permServer.SocketPath(),
		})
		if hookSettings != "" {
			args = append(args, "--settings", hookSettings)
		}
	}

	args = append(args, "-p", prompt)

	cmd := exec.Command("claude", args...)
	cmd.Dir = workingDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start claude: %w", err)
	}

	// Stream output; wait for both goroutines before returning so no
	// data is lost when cmd.Wait() closes the pipes.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamOutput(stdout) }()
	go func() { defer wg.Done(); streamOutput(stderr) }()

	err = cmd.Wait()
	wg.Wait()
	return err
}

func streamOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: stream read error: %v\n", err)
	}
}
