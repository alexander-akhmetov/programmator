package review

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/worksonmyai/programmator/internal/protocol"
)

// reviewResultRegex matches REVIEW_RESULT: blocks in Claude output.
var reviewResultRegex = regexp.MustCompile(`(?s)` + protocol.ReviewResultBlockKey + `:\s*\n(.*?)(?:\n\s*\x60{3}|$)`)

const noStructuredReviewOutputSummary = "No structured review output found"

// ParsedReviewResult is the structured review output.
type ParsedReviewResult struct {
	Issues  []Issue `yaml:"issues"`
	Summary string  `yaml:"summary"`
}

// parseReviewOutput extracts and parses a REVIEW_RESULT block from Claude output.
func parseReviewOutput(output string) ([]Issue, string, error) {
	match := reviewResultRegex.FindStringSubmatch(output)
	if match == nil {
		// No REVIEW_RESULT block found - treat as no issues
		return []Issue{}, noStructuredReviewOutputSummary, nil
	}

	yamlContent := protocol.ReviewResultBlockKey + ":\n" + match[1]
	yamlContent = strings.TrimRight(yamlContent, "`\n ")

	var wrapper struct {
		Result ParsedReviewResult `yaml:"REVIEW_RESULT"`
	}

	if err := yaml.Unmarshal([]byte(yamlContent), &wrapper); err != nil {
		return nil, "", err
	}

	return wrapper.Result.Issues, wrapper.Result.Summary, nil
}

// FormatIssuesMarkdown formats issues as markdown for ticket notes.
func FormatIssuesMarkdown(results []*Result) string {
	var b strings.Builder

	for _, result := range results {
		if result.Error != nil {
			b.WriteString("### ")
			b.WriteString(result.AgentName)
			b.WriteString(" (error)\n")
			b.WriteString("Error: ")
			b.WriteString(result.Error.Error())
			b.WriteString("\n\n")
			continue
		}

		if len(result.Issues) == 0 {
			continue
		}

		b.WriteString("### ")
		b.WriteString(result.AgentName)
		b.WriteString(" (")
		b.WriteString(pluralize(len(result.Issues), "issue", "issues"))
		b.WriteString(")\n\n")

		for _, issue := range result.Issues {
			b.WriteString("- **[")
			b.WriteString(string(issue.Severity))
			b.WriteString("]** ")
			if issue.File != "" {
				b.WriteString("`")
				b.WriteString(issue.File)
				if issue.Line > 0 {
					b.WriteString(":")
					b.WriteString(strconv.Itoa(issue.Line))
					if issue.LineEnd > 0 {
						b.WriteString("-")
						b.WriteString(strconv.Itoa(issue.LineEnd))
					}
				}
				b.WriteString("` - ")
			}
			b.WriteString(issue.Description)
			if issue.Suggestion != "" {
				b.WriteString("\n  - _Suggestion: ")
				b.WriteString(issue.Suggestion)
				b.WriteString("_")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatIssuesYAML formats issues as structured YAML with IDs for validator input.
func FormatIssuesYAML(results []*Result) string {
	type yamlIssue struct {
		ID          string `yaml:"id"`
		File        string `yaml:"file"`
		Line        int    `yaml:"line,omitempty"`
		LineEnd     int    `yaml:"line_end,omitempty"`
		Severity    string `yaml:"severity"`
		Category    string `yaml:"category"`
		Description string `yaml:"description"`
		Suggestion  string `yaml:"suggestion,omitempty"`
		Agent       string `yaml:"agent"`
	}

	totalCount := 0
	for _, res := range results {
		totalCount += len(res.Issues)
	}

	issues := make([]yamlIssue, 0, totalCount)
	for _, res := range results {
		for _, issue := range res.Issues {
			issues = append(issues, yamlIssue{
				ID:          issue.ID,
				File:        issue.File,
				Line:        issue.Line,
				LineEnd:     issue.LineEnd,
				Severity:    string(issue.Severity),
				Category:    issue.Category,
				Description: issue.Description,
				Suggestion:  issue.Suggestion,
				Agent:       res.AgentName,
			})
		}
	}

	data, err := yaml.Marshal(map[string]any{"issues": issues})
	if err != nil {
		return fmt.Sprintf("issues: [] # marshal error: %v", err)
	}
	return string(data)
}

func pluralize(n int, singular, plural string) string { //nolint:unparam // generic helper
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}
