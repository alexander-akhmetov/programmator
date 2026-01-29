package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/worksonmyai/programmator/internal/permission"
)

var (
	runWorkingDir      string
	runSkipPermissions bool
	runAllowPatterns   []string
	runNonInteractive  bool
	runMaxTurns        int
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
}

func runRun(_ *cobra.Command, args []string) error {
	var prompt string

	if len(args) > 0 {
		prompt = strings.Join(args, " ")
	} else {
		// Read from stdin if no args
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			prompt = strings.TrimSpace(string(data))
		}
	}

	if prompt == "" {
		return fmt.Errorf("no prompt provided. Usage: programmator run \"your prompt here\"")
	}

	wd := runWorkingDir
	if wd == "" {
		var err error
		wd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	if runNonInteractive {
		return runClaudePrint(prompt, wd)
	}

	return runClaudeTUI(prompt, wd)
}

func runClaudePrint(prompt, workingDir string) error {
	args := []string{"--print", "-p", prompt}

	if runSkipPermissions {
		args = append([]string{"--dangerously-skip-permissions"}, args...)
	}

	if runMaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", runMaxTurns))
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = workingDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func runClaudeTUI(prompt, workingDir string) error {
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

		go func() { _ = permServer.Serve(ctx) }()
	}

	// Build Claude command
	args := []string{}

	if runSkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	} else if permServer != nil {
		hookSettings := buildRunHookSettings(permServer.SocketPath())
		args = append(args, "--settings", hookSettings)
	}

	if runMaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", runMaxTurns))
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

	// Stream output
	go streamOutput(stdout)
	go streamOutput(stderr)

	return cmd.Wait()
}

func streamOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Println(scanner.Text())
	}
}

func buildRunHookSettings(socketPath string) string {
	exePath, _ := os.Executable()
	return fmt.Sprintf(`{"hooks":{"PreToolUse":[{"type":"command","command":"%s hook","timeout":120000}]},"env":{"PROGRAMMATOR_PERMISSION_SOCKET":"%s"}}`, exePath, socketPath)
}
