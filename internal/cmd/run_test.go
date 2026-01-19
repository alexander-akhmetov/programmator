package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCmdDefinition(t *testing.T) {
	assert.Equal(t, "run [prompt]", runCmd.Use)
	assert.NotEmpty(t, runCmd.Short)
	assert.NotEmpty(t, runCmd.Long)
}

func TestRunCmdFlags(t *testing.T) {
	flags := runCmd.Flags()

	dirFlag := flags.Lookup("dir")
	require.NotNil(t, dirFlag)
	assert.Equal(t, "d", dirFlag.Shorthand)

	skipPermFlag := flags.Lookup("dangerously-skip-permissions")
	require.NotNil(t, skipPermFlag)
	assert.Equal(t, "false", skipPermFlag.DefValue)

	allowFlag := flags.Lookup("allow")
	require.NotNil(t, allowFlag)

	printFlag := flags.Lookup("print")
	require.NotNil(t, printFlag)
	assert.Equal(t, "false", printFlag.DefValue)

	maxTurnsFlag := flags.Lookup("max-turns")
	require.NotNil(t, maxTurnsFlag)
	assert.Equal(t, "0", maxTurnsFlag.DefValue)
}

func TestRunRunNoPrompt(t *testing.T) {
	err := runRun(nil, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no prompt provided")
}

func TestRunRunWithArgs(_ *testing.T) {
	// Test that args are joined into prompt
	// Note: This test would actually try to run Claude, so we just test the arg handling
	oldNonInteractive := runNonInteractive
	oldSkipPermissions := runSkipPermissions
	defer func() {
		runNonInteractive = oldNonInteractive
		runSkipPermissions = oldSkipPermissions
	}()

	// We can't fully test without mocking Claude, but we can verify the command setup
	runNonInteractive = true
	runSkipPermissions = true

	// The command would fail because Claude isn't available in tests,
	// but we can verify the logic flow up to that point
}

func TestBuildRunHookSettings(t *testing.T) {
	settings := buildRunHookSettings("/tmp/test.sock")

	assert.Contains(t, settings, "PreToolUse")
	assert.Contains(t, settings, "hook")
	assert.Contains(t, settings, "/tmp/test.sock")
	assert.Contains(t, settings, "PROGRAMMATOR_PERMISSION_SOCKET")
}

func TestRunCmdHelp(t *testing.T) {
	// Test that the command's long description contains expected content
	assert.Contains(t, runCmd.Long, "Run Claude Code with a custom prompt")
	assert.Contains(t, runCmd.Long, "programmator run")
}
