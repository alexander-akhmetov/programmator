// Package timing provides utilities for measuring and logging startup timing.
package timing

import (
	"fmt"
	"os"
	"time"
)

var (
	enabled   bool
	startTime time.Time
	lastTime  time.Time
)

func init() {
	enabled = os.Getenv("PROGRAMMATOR_DEBUG_TIMING") == "1"
	if enabled {
		startTime = time.Now()
		lastTime = startTime
	}
}

// Log logs a timing checkpoint if PROGRAMMATOR_DEBUG_TIMING=1
func Log(label string) {
	if !enabled {
		return
	}
	now := time.Now()
	sinceLast := now.Sub(lastTime)
	sinceStart := now.Sub(startTime)
	fmt.Fprintf(os.Stderr, "[TIMING] %s: +%dms (total: %dms)\n", label, sinceLast.Milliseconds(), sinceStart.Milliseconds())
	lastTime = now
}
