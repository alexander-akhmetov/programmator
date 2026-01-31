package source

import (
	"sync"

	"github.com/worksonmyai/programmator/internal/domain"
)

// MockSource implements the Source interface for testing.
type MockSource struct {
	mu sync.Mutex

	GetFunc         func(id string) (*domain.WorkItem, error)
	UpdatePhaseFunc func(id, phaseName string) error
	AddNoteFunc     func(id, note string) error
	SetStatusFunc   func(id, status string) error
	TypeFunc        func() string

	GetCalls         []string
	UpdatePhaseCalls []struct{ ID, PhaseName string }
	AddNoteCalls     []struct{ ID, Note string }
	SetStatusCalls   []struct{ ID, Status string }
}

var _ Source = (*MockSource)(nil)

// NewMockSource creates a new MockSource.
func NewMockSource() *MockSource {
	return &MockSource{
		GetCalls:         make([]string, 0),
		UpdatePhaseCalls: make([]struct{ ID, PhaseName string }, 0),
		AddNoteCalls:     make([]struct{ ID, Note string }, 0),
		SetStatusCalls:   make([]struct{ ID, Status string }, 0),
	}
}

// Get retrieves a work item by ID.
func (m *MockSource) Get(id string) (*domain.WorkItem, error) {
	m.mu.Lock()
	m.GetCalls = append(m.GetCalls, id)
	m.mu.Unlock()

	if m.GetFunc != nil {
		return m.GetFunc(id)
	}
	return &domain.WorkItem{ID: id}, nil
}

// UpdatePhase marks a phase as completed.
func (m *MockSource) UpdatePhase(id, phaseName string) error {
	m.mu.Lock()
	m.UpdatePhaseCalls = append(m.UpdatePhaseCalls, struct{ ID, PhaseName string }{id, phaseName})
	m.mu.Unlock()

	if m.UpdatePhaseFunc != nil {
		return m.UpdatePhaseFunc(id, phaseName)
	}
	return nil
}

// AddNote adds a progress note to the work item.
func (m *MockSource) AddNote(id, note string) error {
	m.mu.Lock()
	m.AddNoteCalls = append(m.AddNoteCalls, struct{ ID, Note string }{id, note})
	m.mu.Unlock()

	if m.AddNoteFunc != nil {
		return m.AddNoteFunc(id, note)
	}
	return nil
}

// SetStatus updates the work item's status.
func (m *MockSource) SetStatus(id, status string) error {
	m.mu.Lock()
	m.SetStatusCalls = append(m.SetStatusCalls, struct{ ID, Status string }{id, status})
	m.mu.Unlock()

	if m.SetStatusFunc != nil {
		return m.SetStatusFunc(id, status)
	}
	return nil
}

// Type returns the type of source.
func (m *MockSource) Type() string {
	m.mu.Lock()
	f := m.TypeFunc
	m.mu.Unlock()

	if f != nil {
		return f()
	}
	return "mock"
}
