// Package parser extracts and parses PROGRAMMATOR_STATUS blocks from Claude output.
package parser

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/worksonmyai/programmator/internal/protocol"
)

// Status is an alias for protocol.Status.
type Status = protocol.Status

type ParsedStatus struct {
	PhaseCompleted string   `yaml:"phase_completed"`
	Status         Status   `yaml:"status"`
	FilesChanged   []string `yaml:"files_changed"`
	Summary        string   `yaml:"summary"`
	Error          string   `yaml:"error,omitempty"`
	CommitMade     bool     `yaml:"commit_made,omitempty"`
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

// questionSignalRe matches the QUESTION signal block with JSON payload.
var questionSignalRe = regexp.MustCompile(regexp.QuoteMeta(protocol.SignalQuestion) + `\s*([\s\S]*?)\s*` + regexp.QuoteMeta(protocol.SignalEnd))

// planReadySignalRe matches the PLAN_READY signal block with the plan content.
var planReadySignalRe = regexp.MustCompile(regexp.QuoteMeta(protocol.SignalPlanReady) + `\s*([\s\S]*?)\s*` + regexp.QuoteMeta(protocol.SignalEnd))

// QuestionPayload represents a question signal from Claude during plan creation.
type QuestionPayload struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
	Context  string   `json:"context,omitempty"`
}

// ErrNoQuestionSignal indicates no question signal was found in output.
var ErrNoQuestionSignal = errors.New("no question signal found")

// ErrNoPlanReadySignal indicates no plan ready signal was found in output.
var ErrNoPlanReadySignal = errors.New("no plan ready signal found")

// ParseQuestionPayload extracts a QuestionPayload from output containing QUESTION signal.
// Returns ErrNoQuestionSignal if no question signal is found.
// Returns other error if signal is found but JSON is malformed.
func ParseQuestionPayload(output string) (*QuestionPayload, error) {
	if !strings.Contains(output, protocol.SignalQuestion) {
		return nil, ErrNoQuestionSignal
	}

	matches := questionSignalRe.FindStringSubmatch(output)
	if len(matches) < 2 {
		return nil, errors.New("malformed question signal: missing END marker or empty payload")
	}

	jsonStr := strings.TrimSpace(matches[1])
	if jsonStr == "" {
		return nil, errors.New("malformed question signal: empty JSON payload")
	}

	var payload QuestionPayload
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return nil, errors.New("malformed question signal: invalid JSON: " + err.Error())
	}

	if payload.Question == "" {
		return nil, errors.New("malformed question signal: missing question field")
	}
	if len(payload.Options) == 0 {
		return nil, errors.New("malformed question signal: missing or empty options field")
	}

	return &payload, nil
}

// HasQuestionSignal checks if output contains a question signal.
func HasQuestionSignal(output string) bool {
	return strings.Contains(output, protocol.SignalQuestion)
}

// HasPlanReadySignal checks if output contains a plan ready signal.
func HasPlanReadySignal(output string) bool {
	return strings.Contains(output, protocol.SignalPlanReady)
}

// ParsePlanContent extracts the plan content from a PLAN_READY signal.
// Returns ErrNoPlanReadySignal if no plan ready signal is found.
func ParsePlanContent(output string) (string, error) {
	if !strings.Contains(output, protocol.SignalPlanReady) {
		return "", ErrNoPlanReadySignal
	}

	matches := planReadySignalRe.FindStringSubmatch(output)
	if len(matches) < 2 {
		return "", errors.New("malformed plan ready signal: missing END marker or empty content")
	}

	content := strings.TrimSpace(matches[1])
	if content == "" {
		return "", errors.New("malformed plan ready signal: empty plan content")
	}

	return content, nil
}
