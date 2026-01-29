package input

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminalCollector_SelectWithNumbers(t *testing.T) {
	tests := []struct {
		name     string
		question string
		options  []string
		input    string
		want     string
		wantErr  string
	}{
		{
			name:     "select first option",
			question: "Which color?",
			options:  []string{"Red", "Green", "Blue"},
			input:    "1\n",
			want:     "Red",
		},
		{
			name:     "select middle option",
			question: "Which color?",
			options:  []string{"Red", "Green", "Blue"},
			input:    "2\n",
			want:     "Green",
		},
		{
			name:     "select last option",
			question: "Which color?",
			options:  []string{"Red", "Green", "Blue"},
			input:    "3\n",
			want:     "Blue",
		},
		{
			name:     "invalid number format",
			question: "Which color?",
			options:  []string{"Red", "Green", "Blue"},
			input:    "abc\n",
			wantErr:  "invalid number",
		},
		{
			name:     "out of range too low",
			question: "Which color?",
			options:  []string{"Red", "Green", "Blue"},
			input:    "0\n",
			wantErr:  "selection out of range",
		},
		{
			name:     "out of range too high",
			question: "Which color?",
			options:  []string{"Red", "Green", "Blue"},
			input:    "4\n",
			wantErr:  "selection out of range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &strings.Builder{}
			collector := NewTerminalCollectorWithIO(stdin, stdout)

			got, err := collector.selectWithNumbers(tt.question, tt.options)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			// Verify output contains the question and options
			output := stdout.String()
			assert.Contains(t, output, tt.question)
			for i, opt := range tt.options {
				assert.Contains(t, output, opt)
				assert.Contains(t, output, string(rune('0'+i+1)))
			}
		})
	}
}

func TestTerminalCollector_AskQuestion_NoOptions(t *testing.T) {
	collector := NewTerminalCollector()
	_, err := collector.AskQuestion(context.Background(), "Question?", []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no options provided")
}

func TestTerminalCollector_AskQuestion_NumberedFallback(t *testing.T) {
	// This test verifies the numbered fallback works when fzf is not available
	// We use custom I/O to simulate user input
	stdin := strings.NewReader("2\n")
	stdout := &strings.Builder{}
	collector := NewTerminalCollectorWithIO(stdin, stdout)

	// Force numbered selection by calling selectWithNumbers directly
	// (since fzf might or might not be available in the test environment)
	got, err := collector.selectWithNumbers("Pick one", []string{"A", "B", "C"})
	require.NoError(t, err)
	assert.Equal(t, "B", got)
}
