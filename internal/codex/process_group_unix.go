//go:build !windows

package codex

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// gracefulShutdownDelay is the time to wait between SIGTERM and SIGKILL.
const gracefulShutdownDelay = 100 * time.Millisecond

// processGroupCleanup manages process group lifecycle for graceful shutdown.
type processGroupCleanup struct {
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
	err  error
}

// setupProcessGroup configures command to run in its own process group.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// newProcessGroupCleanup creates a cleanup handler for the given command.
// The command must already be started before calling this.
func newProcessGroupCleanup(cmd *exec.Cmd, cancelCh <-chan struct{}) *processGroupCleanup {
	pg := &processGroupCleanup{
		cmd:  cmd,
		done: make(chan struct{}),
	}
	go pg.watchForCancel(cancelCh)
	return pg
}

func (pg *processGroupCleanup) watchForCancel(cancelCh <-chan struct{}) {
	select {
	case <-cancelCh:
		pg.killProcessGroup()
	case <-pg.done:
	}
}

func (pg *processGroupCleanup) killProcessGroup() {
	process := pg.cmd.Process
	if process == nil {
		return
	}

	pid := process.Pid
	if pid <= 0 {
		log.Printf("[codex] invalid PID %d, skipping process group kill", pid)
		return
	}

	pgid := -pid

	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		log.Printf("[codex] SIGTERM failed for pgid %d: %v", pgid, err)
	}

	time.Sleep(gracefulShutdownDelay)

	if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		log.Printf("[codex] SIGKILL failed for pgid %d: %v", pgid, err)
	}
}

// Wait waits for the command to complete and cleans up resources.
func (pg *processGroupCleanup) Wait() error {
	pg.once.Do(func() {
		pg.err = pg.cmd.Wait()
		close(pg.done)
		if pg.err != nil {
			pg.err = fmt.Errorf("command wait: %w", pg.err)
		}
	})
	return pg.err
}
