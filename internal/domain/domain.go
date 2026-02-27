// Package domain defines the shared model types used across programmator:
// Phase, WorkItem, and their helper methods.
package domain

// Phase represents a single phase or task in a work item.
type Phase struct {
	Name      string
	Completed bool
}

// WorkItem represents a ticket or plan that programmator operates on.
type WorkItem struct {
	// ID is a unique identifier (ticket ID or plan filename).
	ID string
	// Title is the human-readable title of the work item.
	Title string
	// Status is the current status (see protocol.WorkItem* constants).
	Status string
	// Phases are the checkboxed items to complete.
	Phases []Phase
	// RawContent is the full content of the source file.
	RawContent string
	// ValidationCommands are commands to run after each phase (plan files only).
	ValidationCommands []string
}

// CurrentPhaseIndex returns the 0-based index of the first incomplete phase,
// or -1 if all phases are complete or there are no phases.
func (w *WorkItem) CurrentPhaseIndex() int {
	for i := range w.Phases {
		if !w.Phases[i].Completed {
			return i
		}
	}
	return -1
}

// CurrentPhase returns the first incomplete phase, or nil if all are complete.
func (w *WorkItem) CurrentPhase() *Phase {
	for i := range w.Phases {
		if !w.Phases[i].Completed {
			return &w.Phases[i]
		}
	}
	return nil
}

// AllPhasesComplete returns true if all phases are completed.
func (w *WorkItem) AllPhasesComplete() bool {
	for _, p := range w.Phases {
		if !p.Completed {
			return false
		}
	}
	return len(w.Phases) > 0
}

// HasPhases returns true if the work item has any phases defined.
func (w *WorkItem) HasPhases() bool {
	return len(w.Phases) > 0
}
