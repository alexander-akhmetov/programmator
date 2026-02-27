package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestResolveWorkingDir(t *testing.T) {
	t.Run("explicit dir is returned as-is", func(t *testing.T) {
		dir, err := resolveWorkingDir("/some/path")
		assert.NoError(t, err)
		assert.Equal(t, "/some/path", dir)
	})

	t.Run("empty dir returns cwd", func(t *testing.T) {
		dir, err := resolveWorkingDir("")
		assert.NoError(t, err)
		assert.NotEmpty(t, dir)
	})
}

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero", 0, "0s"},
		{"sub-second rounds down", 500 * time.Millisecond, "0s"},
		{"exactly one second", 1 * time.Second, "1s"},
		{"seconds only", 45 * time.Second, "45s"},
		{"just under a minute", 59 * time.Second, "59s"},
		{"exactly one minute", 60 * time.Second, "1m 0s"},
		{"minutes and seconds", 76 * time.Second, "1m 16s"},
		{"several minutes", 5*time.Minute + 30*time.Second, "5m 30s"},
		{"exactly one hour", 60 * time.Minute, "1h 0m"},
		{"hours and minutes", 2*time.Hour + 3*time.Minute, "2h 3m"},
		{"hours drop seconds", 2*time.Hour + 3*time.Minute + 45*time.Second, "2h 3m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatElapsed(tt.duration)
			assert.Equal(t, tt.want, got)
		})
	}
}
