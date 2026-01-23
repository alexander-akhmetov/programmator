// Package debug provides debug logging utilities.
package debug

import (
	"fmt"
	"os"
	"time"
)

var enabled = os.Getenv("PROGRAMMATOR_DEBUG") == "1"

// Logf writes a debug message to stderr if PROGRAMMATOR_DEBUG=1
func Logf(format string, args ...any) {
	if !enabled {
		return
	}
	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[DEBUG %s] %s\n", timestamp, msg)
}

// Enabled returns true if debug logging is enabled
func Enabled() bool {
	return enabled
}
