//go:build !windows

package codex

import (
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupProcessGroup(t *testing.T) {
	cmd := exec.Command("echo", "test")
	setupProcessGroup(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	assert.True(t, cmd.SysProcAttr.Setpgid, "Setpgid should be true")
}

func TestProcessGroupCleanup_Wait(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	setupProcessGroup(cmd)

	require.NoError(t, cmd.Start())

	cancelCh := make(chan struct{})
	pg := newProcessGroupCleanup(cmd, cancelCh)

	err := pg.Wait()
	require.NoError(t, err)

	// Second call should return same result (sync.Once)
	err = pg.Wait()
	require.NoError(t, err)
}

func TestProcessGroupCleanup_CancelKillsProcess(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	setupProcessGroup(cmd)

	require.NoError(t, cmd.Start())

	cancelCh := make(chan struct{})
	pg := newProcessGroupCleanup(cmd, cancelCh)

	// Cancel should trigger kill
	close(cancelCh)

	// Wait should return with an error (process was killed)
	err := pg.Wait()
	require.Error(t, err)
}

func TestKillProcessGroup_NilProcess(_ *testing.T) {
	cmd := &exec.Cmd{}
	pg := &processGroupCleanup{cmd: cmd, done: make(chan struct{})}

	// Should not panic with nil process
	pg.killProcessGroup()
}

func TestProcessGroupCleanup_DoneBeforeCancel(t *testing.T) {
	cmd := exec.Command("true")
	setupProcessGroup(cmd)

	require.NoError(t, cmd.Start())

	cancelCh := make(chan struct{})
	pg := newProcessGroupCleanup(cmd, cancelCh)

	// Process completes normally
	err := pg.Wait()
	require.NoError(t, err)

	// Cancel after done should be safe (watchForCancel exits via done channel)
	close(cancelCh)
	time.Sleep(10 * time.Millisecond)
}

func TestKillProcessGroup_InvalidPID(t *testing.T) {
	// Create a command but manipulate to test invalid PID path
	cmd := exec.Command("echo", "test")
	setupProcessGroup(cmd)
	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Wait())

	pg := &processGroupCleanup{cmd: cmd, done: make(chan struct{})}

	// Process already exited; kill should handle ESRCH gracefully
	pg.killProcessGroup()
}

func TestGracefulShutdownDelay(t *testing.T) {
	// Verify the constant is set to a reasonable value
	assert.Equal(t, 100*time.Millisecond, gracefulShutdownDelay)
}

func TestProcessGroupCleanup_KillSendsSIGTERM(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid

	pg := &processGroupCleanup{cmd: cmd, done: make(chan struct{})}
	pg.killProcessGroup()

	// After kill, process should be dead
	err := cmd.Wait()
	require.Error(t, err, "process should have been killed")
	_ = pid // pid was valid
}
