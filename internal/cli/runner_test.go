package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

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
