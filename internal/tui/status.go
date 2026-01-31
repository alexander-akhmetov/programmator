package tui

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

func runStatus(_ *cobra.Command, _ []string) error {
	path, err := sessionFilePath()
	if err != nil {
		fmt.Println("No active programmator sessions")
		return nil //nolint:nilerr // intentional: no home dir means no session file to check
	}

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
		fmt.Println("No active programmator sessions (corrupted session file, removed)")
		os.Remove(path)
		return nil //nolint:nilerr // intentional: corrupted file is not a user-facing error
	}

	if !isProcessRunning(session.PID) {
		fmt.Println("No active programmator sessions (stale session file, removed)")
		os.Remove(path)
		return nil
	}

	fmt.Println("Active programmator session:")
	fmt.Printf("  Ticket:      %s\n", session.TicketID)
	fmt.Printf("  Working dir: %s\n", session.WorkingDir)
	if startedAt, err := time.Parse(time.RFC3339, session.StartedAt); err == nil {
		elapsed := time.Since(startedAt).Truncate(time.Second)
		fmt.Printf("  Started:     %s (%s ago)\n", startedAt.Format("15:04:05"), elapsed)
	} else {
		fmt.Printf("  Started:     unknown\n")
	}
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
