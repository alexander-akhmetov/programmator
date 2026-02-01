package review

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	require.True(t, cfg.Parallel)
	require.Len(t, cfg.Agents, 9)

	// Verify expected agent names
	names := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		names[i] = a.Name
	}
	require.Contains(t, names, "quality")
	require.Contains(t, names, "quality-2")
	require.Contains(t, names, "security")
	require.Contains(t, names, "implementation")
	require.Contains(t, names, "testing")
	require.Contains(t, names, "simplification")
	require.Contains(t, names, "linter")
	require.Contains(t, names, "claudemd")
	require.Contains(t, names, "codex")
}

func TestConfigFromEnv(t *testing.T) {
	os.Unsetenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS")

	t.Run("default values", func(t *testing.T) {
		cfg := ConfigFromEnv()
		require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	})

	t.Run("env override max iterations", func(t *testing.T) {
		os.Setenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS", "5")
		defer os.Unsetenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS")

		cfg := ConfigFromEnv()
		require.Equal(t, 5, cfg.MaxIterations)
	})

	t.Run("invalid max iterations ignored", func(t *testing.T) {
		os.Setenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS", "invalid")
		defer os.Unsetenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS")

		cfg := ConfigFromEnv()
		require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	})
}

func TestDefaultAgents(t *testing.T) {
	agents := DefaultAgents()

	require.Len(t, agents, 9)

	// Each agent should have a name and focus areas
	for _, a := range agents {
		require.NotEmpty(t, a.Name)
		require.NotEmpty(t, a.Focus)
	}
}
