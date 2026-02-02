// Package main provides the CLI entry point for programmator.
package main

import (
	"os"
	"runtime/debug"

	"github.com/alexander-akhmetov/programmator/internal/tui"
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
	var revision, date string
	dirty := false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			date = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	c := "unknown"
	if len(revision) >= 7 {
		c = revision[:7]
		if dirty {
			c += "-dirty"
		}
	}

	d := "unknown"
	if date != "" {
		d = date
	}
	return c, d
}
