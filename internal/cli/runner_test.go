package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/loop"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

func TestPrintRunSummary(t *testing.T) {
	tests := []struct {
		name     string
		result   *loop.Result
		contains []string
		empty    bool
	}{
		{
			name: "complete",
			result: &loop.Result{
				ExitReason:        safety.ExitReasonComplete,
				Iterations:        5,
				TotalFilesChanged: []string{"a.go", "b.go"},
			},
			contains: []string{"complete", "5", "2"},
		},
		{
			name: "blocked",
			result: &loop.Result{
				ExitReason:  safety.ExitReasonBlocked,
				ExitMessage: "cannot proceed",
				Iterations:  3,
			},
			contains: []string{"blocked", "cannot proceed"},
		},
		{
			name:   "nil result",
			result: nil,
			empty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter(&buf, false, 80, 0)

			printRunSummary(w, tt.result)

			output := buf.String()
			if tt.empty {
				assert.Empty(t, output)
			} else {
				for _, s := range tt.contains {
					assert.Contains(t, output, s)
				}
			}
		})
	}
}

func TestRunConfig_Defaults(t *testing.T) {
	cfg := RunConfig{
		SafetyConfig: safety.Config{MaxIterations: 10},
	}

	assert.Equal(t, 10, cfg.SafetyConfig.MaxIterations)
	assert.Nil(t, cfg.Out)
	assert.False(t, cfg.IsTTY)
}

func TestSnapshotFooterState(t *testing.T) {
	original := &safety.State{
		Iteration:            3,
		ConsecutiveNoChanges: 1,
		TotalFilesChanged: map[string]struct{}{
			"a.go": {},
		},
		StartTime: time.Now().Add(-10 * time.Second),
	}

	snap := snapshotFooterState(original)
	assert.NotNil(t, snap)
	assert.Equal(t, original.Iteration, snap.Iteration)
	assert.Equal(t, original.ConsecutiveNoChanges, snap.ConsecutiveNoChanges)
	assert.Equal(t, original.StartTime, snap.StartTime)
	assert.Len(t, snap.TotalFilesChanged, 1)

	original.TotalFilesChanged["b.go"] = struct{}{}
	assert.Len(t, snap.TotalFilesChanged, 1, "snapshot map must be independent from original")
}

func TestSnapshotFooterWorkItem(t *testing.T) {
	original := &domain.WorkItem{
		ID: "i-123",
		Phases: []domain.Phase{
			{Name: "one", Completed: true},
			{Name: "two", Completed: false},
		},
	}

	snap := snapshotFooterWorkItem(original)
	assert.NotNil(t, snap)
	assert.Equal(t, "i-123", snap.ID)
	assert.Equal(t, original.Phases, snap.Phases)

	original.Phases[0].Name = "changed"
	assert.Equal(t, "one", snap.Phases[0].Name, "snapshot phases must be independent from original")
}
