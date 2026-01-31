// Package main provides the CLI entry point for programmator.
package main

import (
	"os"

	"github.com/worksonmyai/programmator/internal/tui"
)

// Version information set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	tui.SetVersionInfo(version, commit, date)
	if err := tui.Execute(); err != nil {
		os.Exit(1)
	}
}
