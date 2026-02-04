package prompt

import (
	"strings"
	"testing"

	"github.com/alexander-akhmetov/programmator/internal/config"
	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	tests := []struct {
		name        string
		workItem    *domain.WorkItem
		wantSubs    []string
		notWantSubs []string
	}{
		{
			name: "basic prompt with current phase",
			workItem: &domain.WorkItem{
				ID:         "t-123",
				Title:      "Test Ticket",
				RawContent: "Ticket body content",
				Phases: []domain.Phase{
					{Name: "Phase 1", Completed: true},
					{Name: "Phase 2", Completed: false},
				},
			},
			wantSubs: []string{
				"ticket t-123: Test Ticket",
				"Ticket body content",
				"**Phase 2**",
				`phase_completed: "Phase 2"`,
			},
		},
		{
			name: "all phases complete",
			workItem: &domain.WorkItem{
				ID:         "t-789",
				Title:      "Done Ticket",
				RawContent: "All done",
				Phases: []domain.Phase{
					{Name: "Phase 1", Completed: true},
					{Name: "Phase 2", Completed: true},
				},
			},
			wantSubs: []string{
				"All phases complete",
				`phase_completed: "null"`,
			},
		},
		{
			name: "no phases - phaseless mode",
			workItem: &domain.WorkItem{
				ID:         "t-000",
				Title:      "No Phases",
				RawContent: "Empty",
				Phases:     nil,
			},
			wantSubs: []string{
				"ticket t-000: No Phases",
				"STEP 0 - ANNOUNCE",
				"phase_completed: null",
			},
			notWantSubs: []string{
				"Current Phase",
				"All phases complete",
			},
		},
		{
			name: "empty phases - phaseless mode",
			workItem: &domain.WorkItem{
				ID:         "t-001",
				Title:      "Empty Phases",
				RawContent: "Also phaseless",
				Phases:     []domain.Phase{},
			},
			wantSubs: []string{
				"STEP 0 - ANNOUNCE",
			},
			notWantSubs: []string{
				"Current Phase",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(tt.workItem)
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("Build() missing expected substring %q\n\nGot:\n%s", sub, got)
				}
			}
			for _, sub := range tt.notWantSubs {
				if strings.Contains(got, sub) {
					t.Errorf("Build() should not contain substring %q\n\nGot:\n%s", sub, got)
				}
			}
		})
	}
}

func TestBuildPhaseList(t *testing.T) {
	tests := []struct {
		name   string
		phases []domain.Phase
		want   string
	}{
		{
			name:   "empty phases",
			phases: nil,
			want:   "",
		},
		{
			name: "mixed phases",
			phases: []domain.Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: false},
				{Name: "Phase 3", Completed: true},
			},
			want: "- [x] Phase 1\n- [ ] Phase 2\n- [x] Phase 3",
		},
		{
			name: "all incomplete",
			phases: []domain.Phase{
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

func TestNewBuilder(t *testing.T) {
	// Test with nil prompts (uses embedded defaults)
	builder, err := NewBuilder(nil)
	require.NoError(t, err)
	require.NotNil(t, builder)

	// Test building a prompt with the builder
	workItem := &domain.WorkItem{
		ID:         "test-123",
		Title:      "Test Item",
		RawContent: "Test content",
		Phases: []domain.Phase{
			{Name: "Phase 1", Completed: false},
		},
	}

	result, err := builder.Build(workItem)
	require.NoError(t, err)
	assert.Contains(t, result, "test-123")
	assert.Contains(t, result, "Test Item")
	assert.Contains(t, result, "Phase 1")
}

func TestNewBuilder_WithCustomPrompts(t *testing.T) {
	customPrompts := &config.Prompts{
		Phased:      "Custom phased: {{.ID}} - {{.Title}}",
		Phaseless:   "Custom phaseless: {{.ID}}",
		ReviewFirst: "First review: {{.BaseBranch}} iter {{.Iteration}}",
	}

	builder, err := NewBuilder(customPrompts)
	require.NoError(t, err)
	require.NotNil(t, builder)

	// Test phased prompt
	workItem := &domain.WorkItem{
		ID:     "custom-1",
		Title:  "Custom Title",
		Phases: []domain.Phase{{Name: "Phase", Completed: false}},
	}
	result, err := builder.Build(workItem)
	require.NoError(t, err)
	assert.Equal(t, "Custom phased: custom-1 - Custom Title", result)

	// Test phaseless prompt
	phaselessItem := &domain.WorkItem{
		ID:     "custom-2",
		Title:  "Phaseless",
		Phases: nil,
	}
	result, err = builder.Build(phaselessItem)
	require.NoError(t, err)
	assert.Equal(t, "Custom phaseless: custom-2", result)
}

func TestBuilder_BuildReviewFirst(t *testing.T) {
	builder, err := NewBuilder(nil)
	require.NoError(t, err)

	result, err := builder.BuildReviewFirst("main", []string{"file1.go"}, "Critical issue found", 1, false)
	require.NoError(t, err)

	assert.Contains(t, result, "main")
	assert.Contains(t, result, "file1.go")
	assert.Contains(t, result, "Critical issue found")
	assert.Contains(t, result, "1")
	// Verify it's the comprehensive review template (check for unique content)
	assert.Contains(t, result, "CONFIRMED")
	assert.Contains(t, result, "FALSE POSITIVE")
	// Without auto-commit, should not contain commit instructions
	assert.NotContains(t, result, "commit_made")
	assert.NotContains(t, result, "git commit")

	// With auto-commit enabled
	resultAC, err := builder.BuildReviewFirst("main", []string{"file1.go"}, "Critical issue found", 1, true)
	require.NoError(t, err)
	assert.Contains(t, resultAC, "commit_made")
	assert.Contains(t, resultAC, "git commit")
}

func TestNewBuilder_InvalidTemplate(t *testing.T) {
	badPrompts := &config.Prompts{
		Phased:    "{{.Invalid",
		Phaseless: "ok",
	}
	_, err := NewBuilder(badPrompts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse phased template")
}

func TestFormatFilesList(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected string
	}{
		{
			name:     "empty files",
			files:    nil,
			expected: "(no files)",
		},
		{
			name:     "single file",
			files:    []string{"file.go"},
			expected: "  - file.go",
		},
		{
			name:     "multiple files",
			files:    []string{"a.go", "b.go"},
			expected: "  - a.go\n  - b.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFilesList(tt.files)
			assert.Equal(t, tt.expected, got)
		})
	}
}
