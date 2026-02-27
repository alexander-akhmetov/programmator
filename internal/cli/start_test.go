package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartCmdDefinition(t *testing.T) {
	require.Equal(t, "start <ticket-id>", startCmd.Use)
	require.NotEmpty(t, startCmd.Short)
	require.NotEmpty(t, startCmd.Long)
}

func TestStartCmdFlags(t *testing.T) {
	flags := startCmd.Flags()

	dirFlag := flags.Lookup("dir")
	require.NotNil(t, dirFlag)
	require.Equal(t, "d", dirFlag.Shorthand)

	maxIterFlag := flags.Lookup("max-iterations")
	require.NotNil(t, maxIterFlag)

	stagnationFlag := flags.Lookup("stagnation-limit")
	require.NotNil(t, stagnationFlag)

	timeoutFlag := flags.Lookup("timeout")
	require.NotNil(t, timeoutFlag)

	reviewOnlyFlag := flags.Lookup("review-only")
	require.NotNil(t, reviewOnlyFlag)

	autoCommitFlag := flags.Lookup("auto-commit")
	require.NotNil(t, autoCommitFlag)

	moveCompletedFlag := flags.Lookup("move-completed")
	require.NotNil(t, moveCompletedFlag)

	branchFlag := flags.Lookup("branch")
	require.NotNil(t, branchFlag)
}
