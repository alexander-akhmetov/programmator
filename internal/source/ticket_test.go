package source

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/protocol"
	"github.com/worksonmyai/programmator/internal/ticket"
)

// mockTicketClient implements ticket.Client for testing.
type mockTicketClient struct {
	tickets       map[string]*ticket.Ticket
	updatedPhases []struct{ ID, PhaseName string }
	addedNotes    []struct{ ID, Note string }
	statusChanges []struct{ ID, Status string }
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
		return nil, ticket.ErrTicketNotFound
	}
	return t, nil
}

func (m *mockTicketClient) UpdatePhase(id, phaseName string) error {
	if m.returnError != nil {
		return m.returnError
	}
	if phaseName == "" || phaseName == protocol.NullPhase {
		return nil
	}
	m.updatedPhases = append(m.updatedPhases, struct{ ID, PhaseName string }{id, phaseName})
	return nil
}

func (m *mockTicketClient) AddNote(id, note string) error {
	if m.returnError != nil {
		return m.returnError
	}
	m.addedNotes = append(m.addedNotes, struct{ ID, Note string }{id, note})
	return nil
}

func (m *mockTicketClient) SetStatus(id, status string) error {
	if m.returnError != nil {
		return m.returnError
	}
	m.statusChanges = append(m.statusChanges, struct{ ID, Status string }{id, status})
	return nil
}

func TestTicketSource_Get(t *testing.T) {
	mock := newMockTicketClient()
	mock.tickets["test-123"] = &ticket.Ticket{
		ID:     "test-123",
		Title:  "Test Ticket",
		Status: protocol.WorkItemOpen,
		Phases: []domain.Phase{
			{Name: "Phase 1: Design", Completed: true},
			{Name: "Phase 2: Implement", Completed: false},
		},
		RawContent: "# Test Ticket\n\n- [x] Phase 1\n- [ ] Phase 2\n",
	}

	source := NewTicketSource(mock, "")
	item, err := source.Get("test-123")
	require.NoError(t, err)

	assert.Equal(t, "test-123", item.ID)
	assert.Equal(t, "Test Ticket", item.Title)
	assert.Equal(t, protocol.WorkItemOpen, item.Status)
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
	source := NewTicketSource(mock, "")

	err := source.UpdatePhase("test-123", "Phase 1: Design")
	require.NoError(t, err)

	require.Len(t, mock.updatedPhases, 1)
	assert.Equal(t, "test-123", mock.updatedPhases[0].ID)
	assert.Equal(t, "Phase 1: Design", mock.updatedPhases[0].PhaseName)
}

func TestTicketSource_UpdatePhase_Phaseless(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock, "")

	// Empty phase name should be a no-op (phaseless ticket)
	err := source.UpdatePhase("test-123", "")
	require.NoError(t, err)
	assert.Empty(t, mock.updatedPhases)

	// NullPhase phase name should also be a no-op
	err = source.UpdatePhase("test-123", protocol.NullPhase)
	require.NoError(t, err)
	assert.Empty(t, mock.updatedPhases)
}

func TestTicketSource_AddNote(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock, "")

	err := source.AddNote("test-123", "progress: completed task")
	require.NoError(t, err)

	require.Len(t, mock.addedNotes, 1)
	assert.Equal(t, "test-123", mock.addedNotes[0].ID)
	assert.Equal(t, "progress: completed task", mock.addedNotes[0].Note)
}

func TestTicketSource_SetStatus(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock, "")

	err := source.SetStatus("test-123", protocol.WorkItemClosed)
	require.NoError(t, err)

	require.Len(t, mock.statusChanges, 1)
	assert.Equal(t, "test-123", mock.statusChanges[0].ID)
	assert.Equal(t, protocol.WorkItemClosed, mock.statusChanges[0].Status)
}

func TestTicketSource_Type(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock, "")
	assert.Equal(t, TypeTicket, source.Type())
}

func TestTicketSource_Get_NotFound(t *testing.T) {
	mock := newMockTicketClient()
	source := NewTicketSource(mock, "")

	_, err := source.Get("nonexistent")
	require.ErrorIs(t, err, ticket.ErrTicketNotFound)
}

func TestTicketToWorkItem(t *testing.T) {
	tk := &ticket.Ticket{
		ID:     "test-id",
		Title:  "Test Title",
		Status: protocol.WorkItemInProgress,
		Phases: []domain.Phase{
			{Name: "Phase A", Completed: false},
			{Name: "Phase B", Completed: true},
		},
		RawContent: "raw content here",
	}

	item := tk.ToWorkItem()

	assert.Equal(t, "test-id", item.ID)
	assert.Equal(t, "Test Title", item.Title)
	assert.Equal(t, protocol.WorkItemInProgress, item.Status)
	assert.Equal(t, "raw content here", item.RawContent)
	assert.Len(t, item.Phases, 2)

	assert.Equal(t, "Phase A", item.Phases[0].Name)
	assert.False(t, item.Phases[0].Completed)
	assert.Equal(t, "Phase B", item.Phases[1].Name)
	assert.True(t, item.Phases[1].Completed)
}

func TestTicketSource_UpdatePhase_Error(t *testing.T) {
	mock := newMockTicketClient()
	mock.returnError = errors.New("update failed")
	source := NewTicketSource(mock, "")

	err := source.UpdatePhase("test-123", "Phase 1")
	require.ErrorContains(t, err, "update failed")
}

func TestTicketSource_AddNote_Error(t *testing.T) {
	mock := newMockTicketClient()
	mock.returnError = errors.New("note failed")
	source := NewTicketSource(mock, "")

	err := source.AddNote("test-123", "some note")
	require.ErrorContains(t, err, "note failed")
}

func TestTicketSource_SetStatus_Error(t *testing.T) {
	mock := newMockTicketClient()
	mock.returnError = errors.New("status failed")
	source := NewTicketSource(mock, "")

	err := source.SetStatus("test-123", protocol.WorkItemClosed)
	require.ErrorContains(t, err, "status failed")
}

func TestTicketSource_Get_GenericError(t *testing.T) {
	mock := newMockTicketClient()
	mock.returnError = errors.New("permission denied")
	source := NewTicketSource(mock, "")

	_, err := source.Get("test-123")
	require.Error(t, err)
	require.ErrorContains(t, err, "permission denied")
	assert.False(t, errors.Is(err, ticket.ErrTicketNotFound))
}

func TestTicketSource_Get_Phaseless(t *testing.T) {
	mock := newMockTicketClient()
	mock.tickets["phaseless-1"] = &ticket.Ticket{
		ID:         "phaseless-1",
		Title:      "Phaseless Task",
		Status:     protocol.WorkItemOpen,
		Phases:     nil, // No phases
		RawContent: "# Phaseless Task\n\nJust do the thing.\n",
	}

	source := NewTicketSource(mock, "")
	item, err := source.Get("phaseless-1")
	require.NoError(t, err)

	assert.Equal(t, "phaseless-1", item.ID)
	assert.Equal(t, "Phaseless Task", item.Title)
	assert.Empty(t, item.Phases)
	assert.False(t, item.HasPhases())
	assert.Nil(t, item.CurrentPhase())
	assert.False(t, item.AllPhasesComplete())
}
