package review

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
			name: "line ranges parsed correctly",
			input: `
REVIEW_RESULT:
  issues:
    - file: "main.go"
      line: 82-94
      severity: high
      category: "security"
      description: "Sensitive data exposed"
    - file: "util.go"
      line: 42
      severity: medium
      category: "quality"
      description: "Simple line number"
  summary: "Found 2 issues"
`,
			wantIssues: []Issue{
				{
					File:        "main.go",
					Line:        82,
					LineEnd:     94,
					Severity:    SeverityHigh,
					Category:    "security",
					Description: "Sensitive data exposed",
				},
				{
					File:        "util.go",
					Line:        42,
					Severity:    SeverityMedium,
					Category:    "quality",
					Description: "Simple line number",
				},
			},
			wantSummary: "Found 2 issues",
			wantErr:     false,
		},
		{
			name: "quoted string line number",
			input: `
REVIEW_RESULT:
  issues:
    - file: "main.go"
      line: "82"
      severity: medium
      category: "quality"
      description: "Quoted line number"
  summary: "Found 1 issue"
`,
			wantIssues: []Issue{
				{
					File:        "main.go",
					Line:        82,
					Severity:    SeverityMedium,
					Category:    "quality",
					Description: "Quoted line number",
				},
			},
			wantSummary: "Found 1 issue",
			wantErr:     false,
		},
		{
			name: "preserves issue IDs from validator output",
			input: `
REVIEW_RESULT:
  issues:
    - id: "abc123def456"
      file: "main.go"
      line: 42
      severity: high
      category: "bugs"
      description: "Real bug confirmed"
    - id: "xyz789"
      file: "util.go"
      line: 10
      severity: medium
      category: "quality"
      description: "Another issue"
  summary: "Validated 2 of 5 issues"
`,
			wantIssues: []Issue{
				{
					ID:          "abc123def456",
					File:        "main.go",
					Line:        42,
					Severity:    SeverityHigh,
					Category:    "bugs",
					Description: "Real bug confirmed",
				},
				{
					ID:          "xyz789",
					File:        "util.go",
					Line:        10,
					Severity:    SeverityMedium,
					Category:    "quality",
					Description: "Another issue",
				},
			},
			wantSummary: "Validated 2 of 5 issues",
			wantErr:     false,
		},
		{
			name: "unquoted with backslash",
			input: `
REVIEW_RESULT:
  issues:
    - file: main.go
      line: 10
      severity: medium
      category: quality
      description: Use \d+ regex for validation
  summary: Found 1 issue
`,
			wantIssues: []Issue{
				{
					File:        "main.go",
					Line:        10,
					Severity:    SeverityMedium,
					Category:    "quality",
					Description: `Use \d+ regex for validation`,
				},
			},
			wantSummary: "Found 1 issue",
			wantErr:     false,
		},
		{
			name: "single-quoted with backslash",
			input: `
REVIEW_RESULT:
  issues:
    - file: main.go
      line: 10
      severity: medium
      category: quality
      description: 'Use \d+ regex for validation'
  summary: Found 1 issue
`,
			wantIssues: []Issue{
				{
					File:        "main.go",
					Line:        10,
					Severity:    SeverityMedium,
					Category:    "quality",
					Description: `Use \d+ regex for validation`,
				},
			},
			wantSummary: "Found 1 issue",
			wantErr:     false,
		},
		{
			name: "block scalar multiline",
			input: `
REVIEW_RESULT:
  issues:
    - file: main.go
      line: 10
      severity: high
      category: security
      description: |
        This function uses \d+ regex
        which may cause issues
  summary: Found 1 issue
`,
			wantIssues: []Issue{
				{
					File:        "main.go",
					Line:        10,
					Severity:    SeverityHigh,
					Category:    "security",
					Description: "This function uses \\d+ regex\nwhich may cause issues\n",
				},
			},
			wantSummary: "Found 1 issue",
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
				require.Equal(t, want.ID, issues[i].ID)
				require.Equal(t, want.File, issues[i].File)
				require.Equal(t, want.Line, issues[i].Line)
				require.Equal(t, want.LineEnd, issues[i].LineEnd)
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

	t.Run("formats line ranges correctly", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "security",
				Issues: []Issue{
					{
						File:        "main.go",
						Line:        82,
						LineEnd:     94,
						Severity:    SeverityHigh,
						Description: "Issue with range",
					},
				},
			},
		}

		output := FormatIssuesMarkdown(results)
		require.Contains(t, output, "`main.go:82-94`")
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

func TestFormatIssuesYAML(t *testing.T) {
	t.Run("formats issues with IDs and agent", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "abc123", File: "main.go", Line: 42, Severity: SeverityHigh, Category: "bugs", Description: "Error ignored"},
				},
			},
			{
				AgentName: "security",
				Issues: []Issue{
					{ID: "def456", File: "auth.go", Line: 10, Severity: SeverityCritical, Category: "injection", Description: "SQL injection"},
				},
			},
		}

		output := FormatIssuesYAML(results)
		require.Contains(t, output, "id: abc123")
		require.Contains(t, output, "id: def456")
		require.Contains(t, output, "agent: quality")
		require.Contains(t, output, "agent: security")
		require.Contains(t, output, "file: main.go")
		require.Contains(t, output, "file: auth.go")
	})

	t.Run("handles empty results", func(t *testing.T) {
		results := []*Result{
			{AgentName: "quality", Issues: []Issue{}},
		}

		output := FormatIssuesYAML(results)
		require.Contains(t, output, "issues: []")
	})

	t.Run("includes line_end when populated", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "abc", File: "main.go", Line: 10, LineEnd: 20, Severity: SeverityHigh, Category: "bugs", Description: "Range issue"},
				},
			},
		}

		output := FormatIssuesYAML(results)
		require.Contains(t, output, "line: 10")
		require.Contains(t, output, "line_end: 20")
	})

	t.Run("handles special YAML characters in description and suggestion", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "id1", File: "main.go", Line: 10, Severity: SeverityHigh, Category: "bugs", Description: `Error: "foo" not found`, Suggestion: `Use: "bar" instead`},
					{ID: "id2", File: "util.go", Line: 20, Severity: SeverityMedium, Category: "style", Description: "Line with: colon", Suggestion: "Replace with:\n  multiline fix"},
					{ID: "id3", File: "api.go", Line: 30, Severity: SeverityLow, Category: "quality", Description: "Multi\nline\ndescription"},
				},
			},
		}

		output := FormatIssuesYAML(results)

		var parsed struct {
			Issues []struct {
				ID          string `yaml:"id"`
				Description string `yaml:"description"`
				Suggestion  string `yaml:"suggestion"`
			} `yaml:"issues"`
		}
		err := yaml.Unmarshal([]byte(output), &parsed)
		require.NoError(t, err)
		require.Len(t, parsed.Issues, 3)
		require.Equal(t, `Error: "foo" not found`, parsed.Issues[0].Description)
		require.Equal(t, `Use: "bar" instead`, parsed.Issues[0].Suggestion)
		require.Equal(t, "Line with: colon", parsed.Issues[1].Description)
		require.Equal(t, "Replace with:\n  multiline fix", parsed.Issues[1].Suggestion)
		require.Equal(t, "Multi\nline\ndescription", parsed.Issues[2].Description)
		require.Empty(t, parsed.Issues[2].Suggestion)
	})

	t.Run("roundtrip YAML is parseable", func(t *testing.T) {
		results := []*Result{
			{
				AgentName: "quality",
				Issues: []Issue{
					{ID: "abc", File: "main.go", Line: 42, Severity: SeverityHigh, Category: "bugs", Description: "Error ignored"},
				},
			},
		}

		output := FormatIssuesYAML(results)

		var parsed struct {
			Issues []struct {
				ID          string `yaml:"id"`
				File        string `yaml:"file"`
				Line        int    `yaml:"line"`
				Severity    string `yaml:"severity"`
				Category    string `yaml:"category"`
				Description string `yaml:"description"`
				Agent       string `yaml:"agent"`
			} `yaml:"issues"`
		}
		err := yaml.Unmarshal([]byte(output), &parsed)
		require.NoError(t, err)
		require.Len(t, parsed.Issues, 1)
		require.Equal(t, "abc", parsed.Issues[0].ID)
		require.Equal(t, "main.go", parsed.Issues[0].File)
		require.Equal(t, 42, parsed.Issues[0].Line)
		require.Equal(t, "high", parsed.Issues[0].Severity)
		require.Equal(t, "quality", parsed.Issues[0].Agent)
	})
}

func TestPluralize(t *testing.T) {
	require.Equal(t, "1 issue", pluralize(1, "issue", "issues"))
	require.Equal(t, "0 issues", pluralize(0, "issue", "issues"))
	require.Equal(t, "5 issues", pluralize(5, "issue", "issues"))
}
