package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

type sessionInfo struct {
	TicketID   string `json:"ticket_id"`
	WorkingDir string `json:"working_dir"`
	StartedAt  string `json:"started_at"`
	PID        int    `json:"pid"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active loop status",
	Long: `Show the status of any active programmator session.

Displays information about the currently running loop including:
- Ticket ID being worked on
- Working directory
- Start time
- Process ID`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	path := sessionFilePath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No active programmator sessions")
			return nil
		}
		return fmt.Errorf("failed to read session file: %w", err)
	}

	var session sessionInfo
	if err := json.Unmarshal(data, &session); err != nil {
		fmt.Println("No active programmator sessions (corrupted session file)")
		os.Remove(path)
		return nil
	}

	if !isProcessRunning(session.PID) {
		fmt.Println("No active programmator sessions (stale session file)")
		os.Remove(path)
		return nil
	}

	startedAt, _ := time.Parse(time.RFC3339, session.StartedAt)
	elapsed := time.Since(startedAt).Truncate(time.Second)

	fmt.Println("Active programmator session:")
	fmt.Printf("  Ticket:      %s\n", session.TicketID)
	fmt.Printf("  Working dir: %s\n", session.WorkingDir)
	fmt.Printf("  Started:     %s (%s ago)\n", startedAt.Format("15:04:05"), elapsed)
	fmt.Printf("  PID:         %d\n", session.PID)

	return nil
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
