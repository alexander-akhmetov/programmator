package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alexander-akhmetov/programmator/internal/safety"
)

func TestApplySkipPermissions(t *testing.T) {
	tests := []struct {
		name     string
		initial  string
		expected string
	}{
		{
			name:     "empty flags",
			initial:  "",
			expected: "--dangerously-skip-permissions",
		},
		{
			name:     "existing flags without skip",
			initial:  "--verbose",
			expected: "--verbose --dangerously-skip-permissions",
		},
		{
			name:     "already has skip flag",
			initial:  "--dangerously-skip-permissions",
			expected: "--dangerously-skip-permissions",
		},
		{
			name:     "already has skip flag with other flags",
			initial:  "--verbose --dangerously-skip-permissions --other",
			expected: "--verbose --dangerously-skip-permissions --other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := safety.Config{Claude: safety.ClaudeConfig{Flags: tc.initial}}
			applySkipPermissions(&cfg)
			assert.Equal(t, tc.expected, cfg.Claude.Flags)
		})
	}
}

func TestResolveGuardMode(t *testing.T) {
	t.Run("disabled guard mode returns false", func(t *testing.T) {
		cfg := safety.Config{}
		result := resolveGuardMode(false, &cfg)
		assert.False(t, result)
		assert.Empty(t, cfg.Claude.Flags)
	})

	// Note: cannot test dcg found/not found without mocking exec.LookPath,
	// but we can test the disabled case.
}

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
