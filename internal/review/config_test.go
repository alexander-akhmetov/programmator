package review

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	require.True(t, cfg.Parallel)
	require.Len(t, cfg.Agents, 8)

	// Verify expected agent names
	names := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		names[i] = a.Name
	}
	require.Contains(t, names, "error-handling")
	require.Contains(t, names, "logic")
	require.Contains(t, names, "security")
	require.Contains(t, names, "implementation")
	require.Contains(t, names, "testing")
	require.Contains(t, names, "simplification")
	require.Contains(t, names, "linter")
	require.Contains(t, names, "claudemd")
}

func TestDefaultAgents(t *testing.T) {
	agents := DefaultAgents()

	require.Len(t, agents, 8)

	// Each agent should have a name and focus areas
	for _, a := range agents {
		require.NotEmpty(t, a.Name)
		require.NotEmpty(t, a.Focus)
	}
}
