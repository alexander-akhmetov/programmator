package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionFromSettings(t *testing.T) {
	tests := []struct {
		name       string
		settings   []debug.BuildSetting
		wantCommit string
		wantDate   string
	}{
		{
			name:       "empty settings",
			settings:   nil,
			wantCommit: "unknown",
			wantDate:   "unknown",
		},
		{
			name: "full revision and time",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234def5678"},
				{Key: "vcs.time", Value: "2026-01-15T10:00:00Z"},
			},
			wantCommit: "abc1234",
			wantDate:   "2026-01-15T10:00:00Z",
		},
		{
			name: "dirty working tree",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234def5678"},
				{Key: "vcs.time", Value: "2026-01-15T10:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			},
			wantCommit: "abc1234-dirty",
			wantDate:   "2026-01-15T10:00:00Z",
		},
		{
			name: "clean working tree",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234def5678"},
				{Key: "vcs.modified", Value: "false"},
			},
			wantCommit: "abc1234",
			wantDate:   "unknown",
		},
		{
			name: "short revision ignored",
			settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc"},
			},
			wantCommit: "unknown",
			wantDate:   "unknown",
		},
		{
			name: "dirty without revision is not appended",
			settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
			},
			wantCommit: "unknown",
			wantDate:   "unknown",
		},
		{
			name: "modified before revision (real-world alphabetical order)",
			settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.revision", Value: "abc1234def5678"},
				{Key: "vcs.time", Value: "2026-01-15T10:00:00Z"},
			},
			wantCommit: "abc1234-dirty",
			wantDate:   "2026-01-15T10:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCommit, gotDate := versionFromSettings(tt.settings)
			assert.Equal(t, tt.wantCommit, gotCommit)
			assert.Equal(t, tt.wantDate, gotDate)
		})
	}
}
