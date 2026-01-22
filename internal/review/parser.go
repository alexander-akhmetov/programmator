package review

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// reviewResultRegex matches REVIEW_RESULT: blocks in Claude output.
var reviewResultRegex = regexp.MustCompile(`(?s)REVIEW_RESULT:\s*\n(.*?)(?:\n\s*\x60{3}|$)`)

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
		return []Issue{}, "No structured review output found", nil
	}

	yamlContent := "REVIEW_RESULT:\n" + match[1]
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
func FormatIssuesMarkdown(results []*ReviewResult) string {
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
					b.WriteString(itoa(issue.Line))
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

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return itoa(n) + " " + plural
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
