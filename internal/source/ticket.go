package source

import (
	"github.com/alexander-akhmetov/programmator/internal/ticket"
)

// TicketSource adapts a ticket.Client to the Source interface.
type TicketSource struct {
	client ticket.Client
}

var _ Source = (*TicketSource)(nil)

// NewTicketSource creates a new TicketSource with the given client.
// If client is nil, a default CLIClient is created.
func NewTicketSource(client ticket.Client) *TicketSource {
	if client == nil {
		client = ticket.NewClient()
	}
	return &TicketSource{client: client}
}

// Get retrieves a ticket by ID and converts it to a WorkItem.
func (s *TicketSource) Get(id string) (*WorkItem, error) {
	t, err := s.client.Get(id)
	if err != nil {
		return nil, err
	}
	return ticketToWorkItem(t), nil
}

// UpdatePhase marks a phase as completed in the ticket.
func (s *TicketSource) UpdatePhase(id, phaseName string) error {
	return s.client.UpdatePhase(id, phaseName)
}

// AddNote adds a progress note to the ticket.
func (s *TicketSource) AddNote(id, note string) error {
	return s.client.AddNote(id, note)
}

// SetStatus updates the ticket's status.
func (s *TicketSource) SetStatus(id, status string) error {
	return s.client.SetStatus(id, status)
}

// Type returns "ticket".
func (s *TicketSource) Type() string {
	return "ticket"
}

// ticketToWorkItem converts a ticket.Ticket to a WorkItem.
func ticketToWorkItem(t *ticket.Ticket) *WorkItem {
	phases := make([]Phase, len(t.Phases))
	for i, p := range t.Phases {
		phases[i] = Phase{
			Name:      p.Name,
			Completed: p.Completed,
		}
	}

	return &WorkItem{
		ID:         t.ID,
		Title:      t.Title,
		Status:     t.Status,
		Phases:     phases,
		RawContent: t.RawContent,
	}
}
