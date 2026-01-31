// Package source provides a common interface for work sources (tickets and plans).
// This abstraction allows the loop to work with either ticket files or plan files
// as the source of work items and phases.
package source

import (
	"errors"

	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/protocol"
)

// Re-export source type constants for use by source implementations.
const (
	TypePlan   = protocol.SourceTypePlan
	TypeTicket = protocol.SourceTypeTicket
)

// Sentinel errors returned by source implementations.
var (
	// ErrNotFound is returned when a work item, phase, or task cannot be found.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyComplete is returned when a phase or task is already marked complete.
	ErrAlreadyComplete = errors.New("already complete")
)

// --- Capability interfaces ---

// Reader retrieves a work item by identifier.
type Reader interface {
	Get(id string) (*domain.WorkItem, error)
}

// PhaseUpdater marks a phase as completed.
type PhaseUpdater interface {
	UpdatePhase(id, phaseName string) error
}

// StatusUpdater updates the work item's status (e.g. open, in_progress, closed).
type StatusUpdater interface {
	SetStatus(id, status string) error
}

// Noter adds a progress note to the work item.
type Noter interface {
	AddNote(id, note string) error
}

// TypeProvider returns the source type string (e.g. "ticket" or "plan").
type TypeProvider interface {
	Type() string
}

// Mover can relocate a work item to a destination directory.
// Only plan sources support this.
type Mover interface {
	// FilePath returns the current path of the source file.
	FilePath() string
	// MoveTo moves the source file to destDir and returns the new path.
	MoveTo(destDir string) (string, error)
}

// Source is the common interface for ticket and plan sources.
// It composes the core capability interfaces. Implementations may
// additionally satisfy Mover for plan-file relocation.
type Source interface {
	Reader
	PhaseUpdater
	StatusUpdater
	Noter
	TypeProvider
}
