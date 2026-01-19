// Package main provides the CLI entry point for programmator.
package main

import (
	"os"

	"github.com/alexander-akhmetov/programmator/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
