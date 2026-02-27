package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkItem_CurrentPhase(t *testing.T) {
	tests := []struct {
		name   string
		phases []Phase
		want   *Phase
	}{
		{
			name:   "no phases",
			phases: nil,
			want:   nil,
		},
		{
			name:   "all complete",
			phases: []Phase{{Name: "A", Completed: true}},
			want:   nil,
		},
		{
			name:   "first incomplete",
			phases: []Phase{{Name: "A", Completed: true}, {Name: "B", Completed: false}},
			want:   &Phase{Name: "B", Completed: false},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &WorkItem{Phases: tc.phases}
			got := w.CurrentPhase()
			if tc.want == nil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tc.want.Name, got.Name)
			}
		})
	}
}

func TestWorkItem_CurrentPhaseIndex(t *testing.T) {
	tests := []struct {
		name   string
		phases []Phase
		want   int
	}{
		{"no phases", nil, -1},
		{"empty phases", []Phase{}, -1},
		{"all complete", []Phase{{Name: "A", Completed: true}}, -1},
		{"first incomplete", []Phase{{Name: "A", Completed: false}, {Name: "B", Completed: false}}, 0},
		{"second incomplete", []Phase{{Name: "A", Completed: true}, {Name: "B", Completed: false}}, 1},
		{"middle incomplete", []Phase{{Name: "A", Completed: true}, {Name: "B", Completed: false}, {Name: "C", Completed: false}}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &WorkItem{Phases: tc.phases}
			assert.Equal(t, tc.want, w.CurrentPhaseIndex())
		})
	}
}

func TestWorkItem_AllPhasesComplete(t *testing.T) {
	tests := []struct {
		name   string
		phases []Phase
		want   bool
	}{
		{"no phases", nil, false},
		{"all complete", []Phase{{Name: "A", Completed: true}}, true},
		{"one incomplete", []Phase{{Name: "A", Completed: true}, {Name: "B", Completed: false}}, false},
		{"non-contiguous completion", []Phase{{Name: "A", Completed: true}, {Name: "B", Completed: false}, {Name: "C", Completed: true}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &WorkItem{Phases: tc.phases}
			assert.Equal(t, tc.want, w.AllPhasesComplete())
		})
	}
}

func TestWorkItem_HasPhases(t *testing.T) {
	assert.False(t, (&WorkItem{}).HasPhases())
	assert.False(t, (&WorkItem{Phases: []Phase{}}).HasPhases())
	assert.True(t, (&WorkItem{Phases: []Phase{{Name: "A"}}}).HasPhases())
}
