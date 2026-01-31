package source

import (
	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/ticket"
)

// TicketSource adapts a ticket.Client to the Source interface.
type TicketSource struct {
	client ticket.Client
}

var _ Source = (*TicketSource)(nil)

// NewTicketSource creates a new TicketSource with the given client.
// If client is nil, a default CLIClient is created using the given command name.
func NewTicketSource(client ticket.Client, ticketCommand string) *TicketSource {
	if client == nil {
		client = ticket.NewClient(ticketCommand)
	}
	return &TicketSource{client: client}
}

// Get retrieves a ticket by ID and converts it to a WorkItem.
func (s *TicketSource) Get(id string) (*domain.WorkItem, error) {
	t, err := s.client.Get(id)
	if err != nil {
		return nil, err
	}
	return t.ToWorkItem(), nil
}

// UpdatePhase marks a phase as completed in the ticket.
// The underlying client handles phaseless tickets (empty/null phase names).
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
	return TypeTicket
}
