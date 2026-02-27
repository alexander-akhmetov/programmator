// Package protocol defines the cross-package vocabulary for programmator:
// status values, block markers, signal strings, log markers, source type
// identifiers, and the null-phase sentinel.
package protocol

// Status represents the status reported by Claude in a PROGRAMMATOR_STATUS block.
type Status string

const (
	StatusContinue   Status = "CONTINUE"
	StatusDone       Status = "DONE"
	StatusBlocked    Status = "BLOCKED"
	StatusReviewPass Status = "REVIEW_PASS"
	StatusReviewFail Status = "REVIEW_FAIL"
)

func (s Status) String() string { return string(s) }

// IsValid reports whether s is a recognised status value.
func (s Status) IsValid() bool {
	switch s {
	case StatusContinue, StatusDone, StatusBlocked, StatusReviewPass, StatusReviewFail:
		return true
	default:
		return false
	}
}

// Block marker: the key that begins a PROGRAMMATOR_STATUS YAML block.
const StatusBlockKey = "PROGRAMMATOR_STATUS"

// Review result block key.
const ReviewResultBlockKey = "REVIEW_RESULT"

// Source type identifiers returned by Source.Type().
const (
	SourceTypePlan   = "plan"
	SourceTypeTicket = "ticket"
)

// NullPhase is the sentinel value used in the status block when there is no
// current phase (phaseless execution or all phases complete).
const NullPhase = "null"

// Work item status values used by Source.SetStatus and WorkItem.Status.
const (
	WorkItemOpen       = "open"
	WorkItemInProgress = "in_progress"
	WorkItemClosed     = "closed"
)
