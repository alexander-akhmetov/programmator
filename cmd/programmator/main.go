// Package main provides the CLI entry point for programmator.
package main

import (
	"os"

	"github.com/alexander-akhmetov/programmator/internal/cmd"
)

// Version information set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
