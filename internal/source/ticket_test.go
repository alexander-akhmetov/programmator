package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/ticket"
)

// mockTicketClient implements ticket.Client for testing.
type mockTicketClient struct {
	tickets       map[string]*ticket.Ticket
	updatedPhases []string
	addedNotes    []string
	statusChanges []string
	returnError   error
}

func newMockTicketClient() *mockTicketClient {
	return &mockTicketClient{
		tickets: make(map[string]*ticket.Ticket),
	}
}

func (m *mockTicketClient) Get(id string) (*ticket.Ticket, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	t, ok := m.tickets[id]
	if !ok {
		return nil, assert.AnError
	}
	return t, nil
}

func (m *mockTicketClient) UpdatePhase(_, phaseName string) error {
	if m.returnError != nil {
		return m.returnError
	}
	m.updatedPhases = append(m.updatedPhases, phaseName)
	return nil
}

func (m *mockTicketClient) AddNote(_, note string) error {
	if m.returnError != nil {
		return m.returnError
	}
	m.addedNotes = append(m.addedNotes, note)
	return nil
}

func (m *mockTicketClient) SetStatus(_, status string) error {
	if m.returnError != nil {
		return m.returnError
	}
	m.statusChanges = append(m.statusChanges, status)
	return nil
}

func TestTicketSource_Get(t *testing.T) {
	mock := newMockTicketClient()
	mock.tickets["test-123"] = &ticket.Ticket{
		ID:     "test-123",
		Title:  "Test Ticket",
		Status: "open",
		Phases: []ticket.Phase{
			{Name: "Phase 1: Design", Completed: true},
			{Name: "Phase 2: Implement", Completed: false},
		},
		RawContent: "# Test Ticket\n\n- [x] Phase 1\n- [ ] Phase 2\n",
	}

	source := NewTicketSource(mock)
	item, err := source.Get("test-123")
	require.NoError(t, err)

	assert.Equal(t, "test-123", item.ID)
	assert.Equal(t, "Test Ticket", item.Title)
	assert.Equal(t, "open", item.Status)
	assert.Len(t, item.Phases, 2)

	assert.Equal(t, "Phase 1: Design", item.Phases[0].Name)
	assert.True(t, item.Phases[0].Completed)

	assert.Equal(t, "Phase 2: Implement", item.Phases[1].Name)
	assert.False(t, item.Phases[1].Completed)

	// Tickets don't have validation commands
	assert.Empty(t, item.ValidationCommands)
}

func TestTicketSource_UpdatePhase(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock)

	err := source.UpdatePhase("test-123", "Phase 1: Design")
	require.NoError(t, err)

	assert.Equal(t, []string{"Phase 1: Design"}, mock.updatedPhases)
}

func TestTicketSource_UpdatePhase_Phaseless(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock)

	// Empty phase name should be a no-op (phaseless ticket)
	err := source.UpdatePhase("test-123", "")
	require.NoError(t, err)
	assert.Empty(t, mock.updatedPhases)

	// "null" phase name should also be a no-op
	err = source.UpdatePhase("test-123", "null")
	require.NoError(t, err)
	assert.Empty(t, mock.updatedPhases)
}

func TestTicketSource_AddNote(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock)

	err := source.AddNote("test-123", "progress: completed task")
	require.NoError(t, err)

	assert.Equal(t, []string{"progress: completed task"}, mock.addedNotes)
}

func TestTicketSource_SetStatus(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock)

	err := source.SetStatus("test-123", "closed")
	require.NoError(t, err)

	assert.Equal(t, []string{"closed"}, mock.statusChanges)
}

func TestTicketSource_Type(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock)
	assert.Equal(t, "ticket", source.Type())
}

func TestTicketSource_Get_NotFound(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock)

	_, err := source.Get("nonexistent")
	assert.Error(t, err)
}

func TestTicketToWorkItem(t *testing.T) {
	ticket := &ticket.Ticket{
		ID:     "test-id",
		Title:  "Test Title",
		Status: "in_progress",
		Phases: []ticket.Phase{
			{Name: "Phase A", Completed: false},
			{Name: "Phase B", Completed: true},
		},
		RawContent: "raw content here",
	}

	item := ticketToWorkItem(ticket)

	assert.Equal(t, "test-id", item.ID)
	assert.Equal(t, "Test Title", item.Title)
	assert.Equal(t, "in_progress", item.Status)
	assert.Equal(t, "raw content here", item.RawContent)
	assert.Len(t, item.Phases, 2)

	assert.Equal(t, "Phase A", item.Phases[0].Name)
	assert.False(t, item.Phases[0].Completed)
	assert.Equal(t, "Phase B", item.Phases[1].Name)
	assert.True(t, item.Phases[1].Completed)
}

func TestTicketSource_Get_Phaseless(t *testing.T) {
	mock := newMockTicketClient()
	mock.tickets["phaseless-1"] = &ticket.Ticket{
		ID:         "phaseless-1",
		Title:      "Phaseless Task",
		Status:     "open",
		Phases:     nil, // No phases
		RawContent: "# Phaseless Task\n\nJust do the thing.\n",
	}

	source := NewTicketSource(mock)
	item, err := source.Get("phaseless-1")
	require.NoError(t, err)

	assert.Equal(t, "phaseless-1", item.ID)
	assert.Equal(t, "Phaseless Task", item.Title)
	assert.Empty(t, item.Phases)
	assert.False(t, item.HasPhases())
	assert.Nil(t, item.CurrentPhase())
	assert.False(t, item.AllPhasesComplete())
}
