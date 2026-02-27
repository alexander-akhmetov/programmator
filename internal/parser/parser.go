// Package parser extracts and parses PROGRAMMATOR_STATUS blocks from Claude output.
package parser

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

// Status is an alias for protocol.Status.
type Status = protocol.Status

type ParsedStatus struct {
	PhaseCompleted      string   `yaml:"phase_completed"`
	PhaseCompletedIndex *int     `yaml:"phase_completed_index,omitempty"`
	Status              Status   `yaml:"status"`
	FilesChanged        []string `yaml:"files_changed"`
	Summary             string   `yaml:"summary"`
	Error               string   `yaml:"error,omitempty"`
	CommitMade          bool     `yaml:"commit_made,omitempty"`
}

// IsValid checks if the parsed status has valid values.
func (p *ParsedStatus) IsValid() bool {
	if p == nil {
		return false
	}
	return p.Status.IsValid()
}

// statusBlockRegex matches PROGRAMMATOR_STATUS: blocks in Claude output.
// It handles both cases:
// 1. Status block followed by closing backticks (```)
// 2. Status block at end of output with no closing backticks
var statusBlockRegex = regexp.MustCompile(`(?s)` + protocol.StatusBlockKey + `:\s*\n(.*?)(?:\n\s*\x60{3}|$)`)

// Parse extracts and parses a PROGRAMMATOR_STATUS block from Claude output.
// Returns nil, nil if no status block is found.
// Returns nil, error if the status block is malformed.
func Parse(output string) (*ParsedStatus, error) {
	match := statusBlockRegex.FindStringSubmatch(output)
	if match == nil {
		return nil, nil
	}

	yamlContent := protocol.StatusBlockKey + ":\n" + match[1]
	yamlContent = strings.TrimRight(yamlContent, "`\n ")

	var wrapper struct {
		Status ParsedStatus `yaml:"PROGRAMMATOR_STATUS"`
	}

	if err := yaml.Unmarshal([]byte(yamlContent), &wrapper); err != nil {
		return nil, err
	}

	return &wrapper.Status, nil
}

// ParseDirect parses YAML content directly into a ParsedStatus struct.
// This is useful for testing or when the YAML is already extracted.
func ParseDirect(output string) (*ParsedStatus, error) {
	var status ParsedStatus
	if err := yaml.Unmarshal([]byte(output), &status); err != nil {
		return nil, err
	}
	return &status, nil
}
