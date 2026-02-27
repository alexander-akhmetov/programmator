package review

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	require.True(t, cfg.Parallel)
	require.True(t, cfg.ValidateIssues)
	require.True(t, cfg.ValidateSimplifications)
	require.Len(t, cfg.Agents, 9)

	// Verify expected agent names
	names := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		names[i] = a.Name
	}
	require.Contains(t, names, "bug-shallow")
	require.Contains(t, names, "bug-deep")
	require.Contains(t, names, "architect")
	require.Contains(t, names, "simplification")
	require.Contains(t, names, "silent-failures")
	require.Contains(t, names, "claudemd")
	require.Contains(t, names, "type-design")
	require.Contains(t, names, "comments")
	require.Contains(t, names, "tests-and-linters")
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
