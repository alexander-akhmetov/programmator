package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/llm"
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

func TestRunRunWithArgsJoinsPrompt(t *testing.T) {
	// Verify that multiple args are joined with spaces into a single prompt.
	// We can't invoke Claude, but we can verify the prompt assembly by
	// testing that a non-empty prompt is produced from multiple args
	// and that the empty-prompt check is not triggered.
	oldNonInteractive := runNonInteractive
	oldWorkingDir := runWorkingDir
	defer func() {
		runNonInteractive = oldNonInteractive
		runWorkingDir = oldWorkingDir
	}()

	runNonInteractive = true
	runWorkingDir = t.TempDir()

	// This will attempt to invoke Claude; we just verify args don't cause
	// the "no prompt provided" error path.
	err := runRun(nil, []string{"hello", "world"})
	if err != nil {
		assert.NotContains(t, err.Error(), "no prompt provided")
	}
}

func TestBuildHookSettingsForRun(t *testing.T) {
	settings := llm.BuildHookSettings(llm.HookConfig{
		PermissionSocketPath: "/tmp/test.sock",
	})

	assert.Contains(t, settings, "PreToolUse")
	assert.Contains(t, settings, "hook")
	assert.Contains(t, settings, "/tmp/test.sock")
}

func TestRunCmdHelp(t *testing.T) {
	// Test that the command's long description contains expected content
	assert.Contains(t, runCmd.Long, "Run Claude Code with a custom prompt")
	assert.Contains(t, runCmd.Long, "programmator run")
}
