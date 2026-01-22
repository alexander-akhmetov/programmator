package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkItem_CurrentPhase(t *testing.T) {
	tests := []struct {
		name     string
		phases   []Phase
		expected *Phase
	}{
		{
			name:     "no phases",
			phases:   []Phase{},
			expected: nil,
		},
		{
			name: "all completed",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
			expected: nil,
		},
		{
			name: "first incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: false},
				{Name: "Phase 2", Completed: false},
			},
			expected: &Phase{Name: "Phase 1", Completed: false},
		},
		{
			name: "second incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: false},
			},
			expected: &Phase{Name: "Phase 2", Completed: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &WorkItem{Phases: tt.phases}
			got := w.CurrentPhase()
			if tt.expected == nil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, tt.expected.Name, got.Name)
			}
		})
	}
}

func TestWorkItem_AllPhasesComplete(t *testing.T) {
	tests := []struct {
		name     string
		phases   []Phase
		expected bool
	}{
		{
			name:     "no phases",
			phases:   []Phase{},
			expected: false,
		},
		{
			name: "all completed",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
			expected: true,
		},
		{
			name: "one incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &WorkItem{Phases: tt.phases}
			assert.Equal(t, tt.expected, w.AllPhasesComplete())
		})
	}
}

func TestWorkItem_HasPhases(t *testing.T) {
	tests := []struct {
		name     string
		phases   []Phase
		expected bool
	}{
		{
			name:     "no phases",
			phases:   []Phase{},
			expected: false,
		},
		{
			name:     "nil phases",
			phases:   nil,
			expected: false,
		},
		{
			name: "has phases",
			phases: []Phase{
				{Name: "Phase 1", Completed: false},
			},
			expected: true,
		},
		{
			name: "has completed phases",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &WorkItem{Phases: tt.phases}
			assert.Equal(t, tt.expected, w.HasPhases())
		})
	}
}
