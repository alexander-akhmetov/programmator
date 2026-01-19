package ticket

import "sync"

type MockClient struct {
	mu sync.Mutex

	GetFunc         func(id string) (*Ticket, error)
	UpdatePhaseFunc func(id, phaseName string) error
	AddNoteFunc     func(id, note string) error
	SetStatusFunc   func(id, status string) error

	GetCalls         []string
	UpdatePhaseCalls []struct{ ID, PhaseName string }
	AddNoteCalls     []struct{ ID, Note string }
	SetStatusCalls   []struct{ ID, Status string }
}

var _ Client = (*MockClient)(nil)

func NewMockClient() *MockClient {
	return &MockClient{
		GetCalls:         make([]string, 0),
		UpdatePhaseCalls: make([]struct{ ID, PhaseName string }, 0),
		AddNoteCalls:     make([]struct{ ID, Note string }, 0),
		SetStatusCalls:   make([]struct{ ID, Status string }, 0),
	}
}

func (m *MockClient) Get(id string) (*Ticket, error) {
	m.mu.Lock()
	m.GetCalls = append(m.GetCalls, id)
	m.mu.Unlock()

	if m.GetFunc != nil {
		return m.GetFunc(id)
	}
	return &Ticket{ID: id}, nil
}

func (m *MockClient) UpdatePhase(id, phaseName string) error {
	m.mu.Lock()
	m.UpdatePhaseCalls = append(m.UpdatePhaseCalls, struct{ ID, PhaseName string }{id, phaseName})
	m.mu.Unlock()

	if m.UpdatePhaseFunc != nil {
		return m.UpdatePhaseFunc(id, phaseName)
	}
	return nil
}

func (m *MockClient) AddNote(id, note string) error {
	m.mu.Lock()
	m.AddNoteCalls = append(m.AddNoteCalls, struct{ ID, Note string }{id, note})
	m.mu.Unlock()

	if m.AddNoteFunc != nil {
		return m.AddNoteFunc(id, note)
	}
	return nil
}

func (m *MockClient) SetStatus(id, status string) error {
	m.mu.Lock()
	m.SetStatusCalls = append(m.SetStatusCalls, struct{ ID, Status string }{id, status})
	m.mu.Unlock()

	if m.SetStatusFunc != nil {
		return m.SetStatusFunc(id, status)
	}
	return nil
}
