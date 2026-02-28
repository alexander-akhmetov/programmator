package llm

import "github.com/alexander-akhmetov/programmator/internal/protocol"

// TimeoutBlockedStatus returns a PROGRAMMATOR_STATUS block indicating the
// executor invocation timed out. Used by all executors when the context
// deadline is exceeded.
func TimeoutBlockedStatus() string {
	return protocol.StatusBlockKey + `:
  phase_completed: ` + protocol.NullPhase + `
  status: ` + string(protocol.StatusBlocked) + `
  files_changed: []
  summary: "Timeout"
  error: "Executor invocation timed out"`
}
