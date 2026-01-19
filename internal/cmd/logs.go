package cmd

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	logsFollow bool
	logsAll    bool
)

var logsCmd = &cobra.Command{
	Use:   "logs <ticket-id>",
	Short: "Show execution logs",
	Long: `Show execution logs for a ticket.

Displays the progress notes and errors recorded during programmator execution.
These are stored as notes on the ticket itself.

By default, only shows progress and error notes. Use --all to see all notes.`,
	Args: cobra.ExactArgs(1),
	RunE: runLogs,
}

func init() {
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output (not implemented)")
	logsCmd.Flags().BoolVarP(&logsAll, "all", "a", false, "Show all notes, not just progress/error")
}

func runLogs(cmd *cobra.Command, args []string) error {
	ticketID := args[0]

	out, err := exec.Command("ticket", "show", ticketID).Output()
	if err != nil {
		return fmt.Errorf("failed to get ticket %s: %w", ticketID, err)
	}

	content := string(out)

	notesSection := extractNotesSection(content)
	if notesSection == "" {
		fmt.Println("No logs found for ticket", ticketID)
		return nil
	}

	fmt.Printf("Logs for ticket %s:\n", ticketID)
	fmt.Println(strings.Repeat("-", 40))

	lines := strings.Split(notesSection, "\n")
	for _, line := range lines {
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
