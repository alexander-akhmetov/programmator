// Package source provides a common interface for work sources (tickets and plans).
// This abstraction allows the loop to work with either ticket files or plan files
// as the source of work items and phases.
package source

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
	// Status is the current status (e.g., "open", "in_progress", "closed").
	Status string
	// Phases are the checkboxed items to complete.
	Phases []Phase
	// RawContent is the full content of the source file.
	RawContent string
	// ValidationCommands are commands to run after each phase (plan files only).
	ValidationCommands []string
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

// Source is the common interface for ticket and plan sources.
// It provides methods to get, update, and manage work items.
type Source interface {
	// Get retrieves a work item by its identifier.
	// For tickets, this is the ticket ID; for plans, this is the file path.
	Get(id string) (*WorkItem, error)

	// UpdatePhase marks a phase as completed.
	UpdatePhase(id, phaseName string) error

	// AddNote adds a progress note to the work item.
	// For tickets, this uses the ticket CLI; for plans, this may be a no-op or append to the file.
	AddNote(id, note string) error

	// SetStatus updates the work item's status.
	// For tickets, this uses the ticket CLI; for plans, this may be a no-op.
	SetStatus(id, status string) error

	// Type returns the type of source ("ticket" or "plan").
	Type() string
}
