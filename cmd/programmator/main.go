// Package main provides the CLI entry point for programmator.
package main

import (
	"os"
	"runtime/debug"

	"github.com/worksonmyai/programmator/internal/tui"
)

// Version information set via ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	fillVersionFromBuildInfo()
	tui.SetVersionInfo(version, commit, date)
	if err := tui.Execute(); err != nil {
		os.Exit(1)
	}
}

func fillVersionFromBuildInfo() {
	if version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	commit, date = versionFromSettings(info.Settings)
}

func versionFromSettings(settings []debug.BuildSetting) (string, string) {
	c, d := "unknown", "unknown"
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				c = s.Value[:7]
			}
		case "vcs.time":
			d = s.Value
		case "vcs.modified":
			if s.Value == "true" && c != "unknown" {
				c += "-dirty"
			}
		}
	}
	return c, d
}
