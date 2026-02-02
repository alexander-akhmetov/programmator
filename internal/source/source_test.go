package source

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockSource(t *testing.T) {
	mock := &MockSource{}
	mock.GetFunc = func(_ string) (*domain.WorkItem, error) {
		return &domain.WorkItem{
			ID:    "test-1",
			Title: "Test",
			Phases: []domain.Phase{
				{Name: "Phase 1", Completed: false},
			},
		}, nil
	}
	mock.UpdatePhaseFunc = func(_, _ string) error { return nil }
	mock.AddNoteFunc = func(_, _ string) error { return nil }
	mock.SetStatusFunc = func(_, _ string) error { return nil }
	mock.TypeFunc = func() string { return TypeTicket }

	w, err := mock.Get("test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", w.ID)
	assert.Equal(t, 1, len(w.Phases))

	assert.NoError(t, mock.UpdatePhase("test-1", "Phase 1"))
	assert.NoError(t, mock.AddNote("test-1", "note"))
	assert.NoError(t, mock.SetStatus("test-1", protocol.WorkItemOpen))
	assert.Equal(t, TypeTicket, mock.Type())
}

func TestPlanSourceType(t *testing.T) {
	tmpDir := t.TempDir()
	planPath := filepath.Join(tmpDir, "plan.md")
	err := os.WriteFile(planPath, []byte("# Plan: Test\n- [ ] Task 1\n"), 0644)
	require.NoError(t, err)

	s := NewPlanSource(planPath)
	assert.Equal(t, TypePlan, s.Type())
}

// TestCapabilityInterfaces verifies that source implementations satisfy the
// expected capability interfaces (Reader, PhaseUpdater, StatusUpdater, Noter,
// TypeProvider, Mover).
func TestCapabilityInterfaces(t *testing.T) {
	t.Run("PlanSource implements Source and Mover", func(t *testing.T) {
		var s Source = NewPlanSource("/any/path")
		assert.NotNil(t, s)

		// PlanSource also satisfies Mover
		_, ok := s.(Mover)
		assert.True(t, ok, "PlanSource should implement Mover")
	})

	t.Run("TicketSource implements Source but not Mover", func(t *testing.T) {
		var s Source = NewTicketSource(nil, "tk")
		assert.NotNil(t, s)

		_, ok := s.(Mover)
		assert.False(t, ok, "TicketSource should not implement Mover")
	})

	t.Run("MockSource implements Source but not Mover", func(t *testing.T) {
		var s Source = NewMockSource()
		assert.NotNil(t, s)

		_, ok := s.(Mover)
		assert.False(t, ok, "MockSource should not implement Mover")
	})

	t.Run("capability interface subsets", func(_ *testing.T) {
		plan := NewPlanSource("/any/path")
		ticket := NewTicketSource(nil, "tk")
		mock := NewMockSource()

		// All implement Reader
		var _ Reader = plan
		var _ Reader = ticket
		var _ Reader = mock

		// All implement PhaseUpdater
		var _ PhaseUpdater = plan
		var _ PhaseUpdater = ticket
		var _ PhaseUpdater = mock

		// All implement StatusUpdater
		var _ StatusUpdater = plan
		var _ StatusUpdater = ticket
		var _ StatusUpdater = mock

		// All implement Noter
		var _ Noter = plan
		var _ Noter = ticket
		var _ Noter = mock

		// All implement TypeProvider
		var _ TypeProvider = plan
		var _ TypeProvider = ticket
		var _ TypeProvider = mock
	})
}

// TestSentinelErrors verifies that sentinel errors are available and can be
// matched with errors.Is.
func TestSentinelErrors(t *testing.T) {
	assert.True(t, errors.Is(ErrNotFound, ErrNotFound))
	assert.True(t, errors.Is(ErrAlreadyComplete, ErrAlreadyComplete))
	assert.False(t, errors.Is(ErrNotFound, ErrAlreadyComplete))
}
