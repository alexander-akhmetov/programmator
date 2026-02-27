package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/dirs"
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

func sessionFilePath() string {
	return filepath.Join(dirs.StateDir(), "session.json")
}

func writeSessionFile(ticketID, workingDir string) error {
	path := sessionFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.Marshal(sessionInfo{
		TicketID:   ticketID,
		WorkingDir: workingDir,
		StartedAt:  time.Now().Format(time.RFC3339),
		PID:        os.Getpid(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

func removeSessionFile() {
	os.Remove(sessionFilePath())
}
