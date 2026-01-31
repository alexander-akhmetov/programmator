//go:build windows

package codex

import (
	"fmt"
	"os/exec"
	"sync"
)

// processGroupCleanup manages process lifecycle on Windows.
// Windows doesn't support Unix process groups, so this only kills the direct process.
type processGroupCleanup struct {
	cmd  *exec.Cmd
	done chan struct{}
	once sync.Once
	err  error
}

func setupProcessGroup(_ *exec.Cmd) {}

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
		pg.killProcess()
	case <-pg.done:
	}
}

func (pg *processGroupCleanup) killProcess() {
	process := pg.cmd.Process
	if process == nil {
		return
	}
	_ = process.Kill()
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
