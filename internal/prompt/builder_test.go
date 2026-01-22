package prompt

import (
	"strings"
	"testing"

	"github.com/alexander-akhmetov/programmator/internal/source"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name     string
		workItem *source.WorkItem
		notes    []string
		wantSubs []string
	}{
		{
			name: "basic prompt with current phase",
			workItem: &source.WorkItem{
				ID:         "t-123",
				Title:      "Test Ticket",
				RawContent: "Ticket body content",
				Phases: []source.Phase{
					{Name: "Phase 1", Completed: true},
					{Name: "Phase 2", Completed: false},
				},
			},
			notes: nil,
			wantSubs: []string{
				"ticket t-123: Test Ticket",
				"Ticket body content",
				"(No previous notes)",
				"**Phase 2**",
				`phase_completed: "Phase 2"`,
			},
		},
		{
			name: "prompt with notes",
			workItem: &source.WorkItem{
				ID:         "t-456",
				Title:      "Another Ticket",
				RawContent: "Body here",
				Phases: []source.Phase{
					{Name: "Phase 1", Completed: false},
				},
			},
			notes: []string{
				"[iter 0] Completed setup",
				"[iter 1] Fixed bug",
			},
			wantSubs: []string{
				"ticket t-456: Another Ticket",
				"- [iter 0] Completed setup",
				"- [iter 1] Fixed bug",
				"**Phase 1**",
			},
		},
		{
			name: "all phases complete",
			workItem: &source.WorkItem{
				ID:         "t-789",
				Title:      "Done Ticket",
				RawContent: "All done",
				Phases: []source.Phase{
					{Name: "Phase 1", Completed: true},
					{Name: "Phase 2", Completed: true},
				},
			},
			notes: nil,
			wantSubs: []string{
				"All phases complete",
				`phase_completed: "null"`,
			},
		},
		{
			name: "no phases",
			workItem: &source.WorkItem{
				ID:         "t-000",
				Title:      "No Phases",
				RawContent: "Empty",
				Phases:     nil,
			},
			notes: nil,
			wantSubs: []string{
				"All phases complete",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(tt.workItem, tt.notes)
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("Build() missing substring %q\n\nGot:\n%s", sub, got)
				}
			}
		})
	}
}

func TestBuildPhaseList(t *testing.T) {
	tests := []struct {
		name   string
		phases []source.Phase
		want   string
	}{
		{
			name:   "empty phases",
			phases: nil,
			want:   "",
		},
		{
			name: "mixed phases",
			phases: []source.Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: false},
				{Name: "Phase 3", Completed: true},
			},
			want: "- [x] Phase 1\n- [ ] Phase 2\n- [x] Phase 3",
		},
		{
			name: "all incomplete",
			phases: []source.Phase{
				{Name: "First", Completed: false},
				{Name: "Second", Completed: false},
			},
			want: "- [ ] First\n- [ ] Second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPhaseList(tt.phases)
			if got != tt.want {
				t.Errorf("BuildPhaseList() = %q, want %q", got, tt.want)
			}
		})
	}
}
