package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alexander-akhmetov/programmator/internal/llm"
)

func TestApplySkipPermissions(t *testing.T) {
	tests := []struct {
		name     string
		initial  []string
		expected []string
	}{
		{
			name:     "empty flags",
			initial:  nil,
			expected: []string{"--dangerously-skip-permissions"},
		},
		{
			name:     "existing flags without skip",
			initial:  []string{"--verbose"},
			expected: []string{"--verbose", "--dangerously-skip-permissions"},
		},
		{
			name:     "already has skip flag",
			initial:  []string{"--dangerously-skip-permissions"},
			expected: []string{"--dangerously-skip-permissions"},
		},
		{
			name:     "already has skip flag with other flags",
			initial:  []string{"--verbose", "--dangerously-skip-permissions", "--other"},
			expected: []string{"--verbose", "--dangerously-skip-permissions", "--other"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := llm.ExecutorConfig{ExtraFlags: tc.initial}
			applySkipPermissions(&cfg)
			assert.Equal(t, tc.expected, cfg.ExtraFlags)
		})
	}
}

func TestResolveGuardMode(t *testing.T) {
	t.Run("disabled guard mode returns false", func(t *testing.T) {
		cfg := llm.ExecutorConfig{}
		result := resolveGuardMode(false, &cfg)
		assert.False(t, result)
		assert.Empty(t, cfg.ExtraFlags)
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
