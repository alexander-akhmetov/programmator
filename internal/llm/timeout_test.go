package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestTimeoutBlockedStatus(t *testing.T) {
	result := TimeoutBlockedStatus()
	assert.Contains(t, result, protocol.StatusBlockKey)
	assert.Contains(t, result, string(protocol.StatusBlocked))
	assert.Contains(t, result, protocol.NullPhase)
	assert.Contains(t, result, "Timeout")
	assert.Contains(t, result, "timed out")
}
