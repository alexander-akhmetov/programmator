package review

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseReviewOutput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantIssues  []Issue
		wantSummary string
		wantErr     bool
	}{
		{
			name: "valid output with issues",
			input: `Some text before

REVIEW_RESULT:
  issues:
    - file: "main.go"
      line: 42
      severity: high
      category: "error handling"
      description: "Error is ignored"
      suggestion: "Handle the error"
  summary: "Found 1 issue"
` + "```",
			wantIssues: []Issue{
				{
					File:        "main.go",
					Line:        42,
					Severity:    SeverityHigh,
					Category:    "error handling",
					Description: "Error is ignored",
					Suggestion:  "Handle the error",
				},
			},
			wantSummary: "Found 1 issue",
			wantErr:     false,
		},
		{
			name: "valid output no issues",
			input: `
REVIEW_RESULT:
  issues: []
  summary: "No issues found"
`,
			wantIssues:  []Issue{},
			wantSummary: "No issues found",
			wantErr:     false,
		},
		{
			name: "multiple issues",
			input: `
REVIEW_RESULT:
  issues:
    - file: "a.go"
      severity: critical
      category: "security"
      description: "SQL injection"
    - file: "b.go"
      line: 10
      severity: medium
      category: "performance"
      description: "Slow loop"
  summary: "Found 2 issues"
`,
			wantIssues: []Issue{
				{
					File:        "a.go",
					Severity:    SeverityCritical,
					Category:    "security",
					Description: "SQL injection",
				},
				{
					File:        "b.go",
					Line:        10,
					Severity:    SeverityMedium,
					Category:    "performance",
					Description: "Slow loop",
				},
			},
			wantSummary: "Found 2 issues",
			wantErr:     false,
		},
		{
			name:        "no REVIEW_RESULT block",
			input:       "Just some random output without the block",
			wantIssues:  []Issue{},
			wantSummary: "No structured review output found",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, summary, err := parseReviewOutput(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantSummary, summary)
			require.Len(t, issues, len(tt.wantIssues))

			for i, want := range tt.wantIssues {
				require.Equal(t, want.File, issues[i].File)
				require.Equal(t, want.Line, issues[i].Line)
				require.Equal(t, want.Severity, issues[i].Severity)
				require.Equal(t, want.Category, issues[i].Category)
				require.Equal(t, want.Description, issues[i].Description)
			}
		})
	}
}

func TestFormatIssuesMarkdown(t *testing.T) {
	t.Run("formats issues correctly", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{
						File:        "main.go",
						Line:        42,
						Severity:    SeverityHigh,
						Category:    "error handling",
						Description: "Error ignored",
						Suggestion:  "Handle it",
					},
				},
			},
		}

		output := FormatIssuesMarkdown(results)
		require.Contains(t, output, "### quality")
		require.Contains(t, output, "1 issue")
		require.Contains(t, output, "[high]")
		require.Contains(t, output, "`main.go:42`")
		require.Contains(t, output, "Error ignored")
		require.Contains(t, output, "_Suggestion: Handle it_")
	})

	t.Run("handles multiple agents", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{File: "a.go", Severity: SeverityLow, Description: "Issue 1"},
				},
			},
			{
				AgentName: "security",
				Issues: []Issue{
					{File: "b.go", Severity: SeverityCritical, Description: "Issue 2"},
				},
			},
		}

		output := FormatIssuesMarkdown(results)
		require.Contains(t, output, "### quality")
		require.Contains(t, output, "### security")
	})

	t.Run("handles error in result", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Error:     error(nil),
				Issues:    []Issue{},
			},
		}

		output := FormatIssuesMarkdown(results)
		require.Empty(t, output)
	})

	t.Run("skips agents with no issues", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues:    []Issue{},
			},
		}

		output := FormatIssuesMarkdown(results)
		require.Empty(t, output)
	})
}

func TestPluralize(t *testing.T) {
	require.Equal(t, "1 issue", pluralize(1, "issue", "issues"))
	require.Equal(t, "0 issues", pluralize(0, "issue", "issues"))
	require.Equal(t, "5 issues", pluralize(5, "issue", "issues"))
}
