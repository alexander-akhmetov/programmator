// Package cli implements the command-line interface for programmator.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// SetVersionInfo sets the version information for the CLI.
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
	rootCmd.Version = fmt.Sprintf("%s (%s, %s)", version, commit, date)
}

var rootCmd = &cobra.Command{
	Use:   "programmator",
	Short: "Ticket-driven autonomous Claude Code loop orchestrator",
	Long: `Programmator reads a ticket, identifies the current phase, invokes Claude Code
with a structured prompt, parses the response, and loops until all phases are
complete or safety limits are reached.`,
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(planCmd)
}
