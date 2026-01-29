package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/progress"
	"github.com/alexander-akhmetov/programmator/internal/source"
)

var (
	logsFollow bool
	logsAll    bool
	logsList   bool
	logsRecent int
)

var logsCmd = &cobra.Command{
	Use:   "logs [source-id]",
	Short: "Show execution logs",
	Long: `Show execution logs for a ticket or plan.

For tickets, displays progress notes from the ticket itself.
For plans (or any source), displays logs from ~/.programmator/logs/.

Options:
  --list, -l       List recent log files
  --follow, -f     Tail the active or most recent log file
  --recent N       Show last N log files (default: 10)
  --all, -a        Show all notes (tickets only)

Examples:
  programmator logs pro-1234           # Show ticket notes
  programmator logs ./plans/my-plan.md # Show plan log files
  programmator logs -l                 # List all recent logs
  programmator logs -f                 # Follow active session
  programmator logs my-plan -f         # Follow specific source log`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output in real-time")
	logsCmd.Flags().BoolVarP(&logsAll, "all", "a", false, "Show all notes, not just progress/error (tickets only)")
	logsCmd.Flags().BoolVarP(&logsList, "list", "l", false, "List recent log files")
	logsCmd.Flags().IntVar(&logsRecent, "recent", 10, "Number of recent logs to show")
}

func runLogs(_ *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// List mode: show recent log files
	if logsList {
		return listLogs(cfg.LogsDir, "")
	}

	// No source ID provided
	if len(args) == 0 {
		if logsFollow {
			// Follow the most recent active log
			return followLogs(cfg.LogsDir, "")
		}
		// Show usage
		return listLogs(cfg.LogsDir, "")
	}

	sourceID := args[0]

	// Determine source type
	if source.IsPlanPath(sourceID) {
		// Plan: use progress log files
		if logsFollow {
			return followLogs(cfg.LogsDir, sourceID)
		}
		return showPlanLogs(cfg.LogsDir, sourceID)
	}

	// Ticket: show notes from ticket
	if logsFollow {
		// For tickets with --follow, use progress logs instead
		return followLogs(cfg.LogsDir, sourceID)
	}
	return showTicketLogs(sourceID)
}

// listLogs lists recent log files.
func listLogs(logsDir, filter string) error {
	logs, err := progress.FindLogs(logsDir, filter)
	if err != nil {
		return fmt.Errorf("failed to find logs: %w", err)
	}

	if len(logs) == 0 {
		fmt.Println("No log files found.")
		if logsDir == "" {
			home, _ := os.UserHomeDir()
			fmt.Printf("Log directory: %s/.programmator/logs/\n", home)
		} else {
			fmt.Printf("Log directory: %s\n", logsDir)
		}
		return nil
	}

	fmt.Printf("Recent log files (showing %d):\n", min(logsRecent, len(logs)))
	fmt.Println(strings.Repeat("-", 60))

	for i, lf := range logs {
		if i >= logsRecent {
			break
		}
		status := ""
		if lf.IsActive {
			status = " [ACTIVE]"
		}
		fmt.Printf("  %s  %-30s%s\n",
			lf.Timestamp.Format("2006-01-02 15:04:05"),
			lf.SourceID,
			status,
		)
		fmt.Printf("    %s\n", lf.Path)
	}

	return nil
}

// showPlanLogs shows the most recent log file for a plan.
func showPlanLogs(logsDir, sourceID string) error {
	lf, err := progress.FindLatestLog(logsDir, sourceID)
	if err != nil {
		return fmt.Errorf("failed to find log: %w", err)
	}
	if lf == nil {
		fmt.Printf("No logs found for source: %s\n", sourceID)
		fmt.Println("Tip: Use 'programmator logs -l' to list all logs")
		return nil
	}

	fmt.Printf("Log for %s (%s):\n", lf.SourceID, lf.Timestamp.Format("2006-01-02 15:04:05"))
	if lf.IsActive {
		fmt.Println("[ACTIVE SESSION]")
	}
	fmt.Println(strings.Repeat("-", 60))

	// Read and print the log file
	data, err := os.ReadFile(lf.Path)
	if err != nil {
		return fmt.Errorf("failed to read log: %w", err)
	}
	fmt.Print(string(data))

	return nil
}

// followLogs tails the active or most recent log file.
func followLogs(logsDir, sourceID string) error {
	// First check for an active log
	lf, err := progress.FindActiveLog(logsDir, sourceID)
	if err != nil {
		return fmt.Errorf("failed to find active log: %w", err)
	}

	if lf == nil {
		// Fall back to most recent log
		lf, err = progress.FindLatestLog(logsDir, sourceID)
		if err != nil {
			return fmt.Errorf("failed to find log: %w", err)
		}
		if lf == nil {
			fmt.Println("No logs found to follow.")
			return nil
		}
		fmt.Printf("No active session found. Following most recent log: %s\n", lf.SourceID)
	} else {
		fmt.Printf("Following active session: %s\n", lf.SourceID)
	}
	fmt.Println(strings.Repeat("-", 60))

	// Use tail -f for following
	cmd := exec.Command("tail", "-f", lf.Path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start tail: %w", err)
	}

	// Wait for command to complete
	return cmd.Wait()
}

// showTicketLogs shows logs from ticket notes.
func showTicketLogs(ticketID string) error {
	out, err := exec.Command("ticket", "show", ticketID).Output()
	if err != nil {
		return fmt.Errorf("failed to get ticket %s: %w", ticketID, err)
	}

	content := string(out)

	notesSection := extractNotesSection(content)
	if notesSection == "" {
		fmt.Println("No logs found for ticket", ticketID)
		fmt.Println("Tip: Use 'programmator logs -l' to list progress log files")
		return nil
	}

	fmt.Printf("Logs for ticket %s:\n", ticketID)
	fmt.Println(strings.Repeat("-", 40))

	for line := range strings.SplitSeq(notesSection, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "## Notes" {
			continue
		}

		if logsAll {
			fmt.Println(line)
			continue
		}

		if isProgrammatorLogLine(line) {
			fmt.Println(formatLogLine(line))
		}
	}

	return nil
}

func extractNotesSection(content string) string {
	idx := strings.Index(content, "## Notes")
	if idx == -1 {
		return ""
	}

	section := content[idx:]

	nextSection := regexp.MustCompile(`\n## [A-Z]`)
	if loc := nextSection.FindStringIndex(section[9:]); loc != nil {
		section = section[:loc[0]+9]
	}

	return section
}

func isProgrammatorLogLine(line string) bool {
	prefixes := []string{
		"progress:",
		"error:",
		"[iter ",
	}

	for _, prefix := range prefixes {
		if strings.Contains(strings.ToLower(line), prefix) {
			return true
		}
	}

	return false
}

func formatLogLine(line string) string {
	scanner := bufio.NewScanner(strings.NewReader(line))
	scanner.Scan()
	text := scanner.Text()

	if strings.HasPrefix(text, "**") {
		dateEnd := strings.Index(text[2:], "**")
		if dateEnd > 0 {
			timestamp := text[2 : 2+dateEnd]
			rest := strings.TrimSpace(text[4+dateEnd:])
			return fmt.Sprintf("[%s] %s", timestamp, rest)
		}
	}

	return text
}
