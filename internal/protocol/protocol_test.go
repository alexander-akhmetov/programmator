package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatusString(t *testing.T) {
	assert.Equal(t, "CONTINUE", StatusContinue.String())
	assert.Equal(t, "DONE", StatusDone.String())
	assert.Equal(t, "BLOCKED", StatusBlocked.String())
}

func TestStatusIsValid(t *testing.T) {
	tests := []struct {
		status Status
		valid  bool
	}{
		{StatusContinue, true},
		{StatusDone, true},
		{StatusBlocked, true},
		{StatusReviewPass, true},
		{StatusReviewFail, true},
		{Status("UNKNOWN"), false},
		{Status(""), false},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.valid, tc.status.IsValid())
		})
	}
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "PROGRAMMATOR_STATUS", StatusBlockKey)
	assert.Equal(t, "REVIEW_RESULT", ReviewResultBlockKey)
	assert.Equal(t, "null", NullPhase)
	assert.Equal(t, "plan", SourceTypePlan)
	assert.Equal(t, "ticket", SourceTypeTicket)
	assert.Equal(t, "open", WorkItemOpen)
	assert.Equal(t, "in_progress", WorkItemInProgress)
	assert.Equal(t, "closed", WorkItemClosed)
}
