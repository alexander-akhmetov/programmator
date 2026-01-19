// Package cmd implements the CLI commands for programmator.
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "programmator",
	Short: "Ticket-driven autonomous Claude Code loop orchestrator",
	Long: `Programmator reads a ticket, identifies the current phase, invokes Claude Code
with a structured prompt, parses the response, and loops until all phases are
complete or safety limits are reached.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logsCmd)
}
