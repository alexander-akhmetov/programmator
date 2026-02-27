// Package ticket provides a client for interacting with the ticket CLI.
package ticket

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

// Sentinel errors for ticket operations.
var (
	// ErrTicketNotFound is returned when a ticket file cannot be found.
	ErrTicketNotFound = errors.New("ticket not found")
	// ErrPhaseNotFound is returned when a phase cannot be found in the ticket.
	ErrPhaseNotFound = errors.New("phase not found")
)

type Ticket struct {
	ID          string
	Title       string
	Status      string
	Priority    int
	Type        string
	Description string
	Phases      []domain.Phase
	RawContent  string
}

type Client interface {
	Get(id string) (*Ticket, error)
	UpdatePhase(id, phaseName string) error
	AddNote(id, note string) error
	SetStatus(id, status string) error
}

type CLIClient struct {
	ticketsDir string
	command    string
}

var _ Client = (*CLIClient)(nil)

var validIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]*$`)

// ValidateID checks that a ticket ID contains only safe characters.
func ValidateID(id string) error {
	if !validIDRe.MatchString(id) {
		return fmt.Errorf("%w: %s", ErrTicketNotFound, id)
	}
	return nil
}

func NewClient(command string) *CLIClient {
	dir := os.Getenv("TICKETS_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		dir = filepath.Join(home, ".tickets")
	}
	if command == "" {
		command = "tk"
	}
	return &CLIClient{ticketsDir: dir, command: command}
}

func (c *CLIClient) Get(id string) (*Ticket, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	out, err := exec.Command(c.command, "show", id).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %s", ErrTicketNotFound, id, strings.TrimSpace(string(out)))
	}

	return parseTicket(id, string(out))
}

func (c *CLIClient) UpdatePhase(id string, phaseName string) error {
	if err := ValidateID(id); err != nil {
		return err
	}

	// For phaseless tickets, there's nothing to update
	if phaseName == "" || phaseName == protocol.NullPhase {
		return nil
	}

	// Find the ticket file
	filePath, err := c.findTicketFile(id)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read ticket file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	result := updatePhaseLines(lines, normalizePhase(phaseName))
	if !result.found {
		return fmt.Errorf("%w: %s", ErrPhaseNotFound, phaseName)
	}
	if result.alreadyDone {
		return nil
	}

	return writeFileAtomically(filePath, []byte(strings.Join(lines, "\n")))
}

type phaseUpdateResult struct {
	found       bool
	alreadyDone bool
}

func updatePhaseLines(lines []string, normalizedPhase string) phaseUpdateResult {
	return updatePhaseInCheckboxes(lines, normalizedPhase)
}

func updatePhaseInCheckboxes(lines []string, normalizedPhase string) phaseUpdateResult {
	for i, line := range lines {
		match := phaseRegex.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		existingPhase := normalizePhase(match[2])
		if !phaseMatches(existingPhase, normalizedPhase) {
			continue
		}

		if match[1] != " " {
			return phaseUpdateResult{found: true, alreadyDone: true}
		}

		lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
		return phaseUpdateResult{found: true}
	}
	return phaseUpdateResult{}
}

func phaseMatches(existingPhase, normalizedPhase string) bool {
	return existingPhase == normalizedPhase ||
		strings.Contains(existingPhase, normalizedPhase) ||
		strings.Contains(normalizedPhase, existingPhase)
}

// Write atomically: temp file + rename to avoid data loss on partial write.
// Preserve original file permissions on the temp file.
func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat original file: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".ticket-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, fi.Mode()); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func (c *CLIClient) findTicketFile(id string) (string, error) {
	path := filepath.Clean(filepath.Join(c.ticketsDir, id+".md"))
	dir := filepath.Clean(c.ticketsDir)
	if !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %s", ErrTicketNotFound, id)
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("%w: %s", ErrTicketNotFound, id)
}

var normalizePrefixRegex = regexp.MustCompile(`^(phase|step)\s*\d+[:.]\s*`)

func normalizePhase(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	s = normalizePrefixRegex.ReplaceAllString(s, "")
	return s
}

func (c *CLIClient) AddNote(id string, note string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	out, err := exec.Command(c.command, "add-note", id, note).CombinedOutput()
	if err != nil {
		return fmt.Errorf("add note to ticket %s: %s: %w", id, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func (c *CLIClient) SetStatus(id string, status string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	switch status {
	case protocol.WorkItemOpen, protocol.WorkItemInProgress, protocol.WorkItemClosed:
		// valid
	default:
		return fmt.Errorf("invalid status: %s", status)
	}
	out, err := exec.Command(c.command, "set-status", id, status).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set status for ticket %s: %s: %w", id, strings.TrimSpace(string(out)), err)
	}
	return nil
}

func parseTicket(id string, content string) (*Ticket, error) {
	ticket := &Ticket{
		ID:         id,
		RawContent: content,
	}

	// Parse YAML frontmatter
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			var frontmatter map[string]any
			if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err == nil {
				if title, ok := frontmatter["title"].(string); ok {
					ticket.Title = title
				}
				if status, ok := frontmatter["status"].(string); ok {
					ticket.Status = status
				}
				if priority, ok := frontmatter["priority"].(int); ok {
					ticket.Priority = priority
				}
				if typ, ok := frontmatter["type"].(string); ok {
					ticket.Type = typ
				}
			}
		}
	}

	// If no title in frontmatter, extract from first # heading
	if ticket.Title == "" {
		if matches := titleRegex.FindStringSubmatch(content); len(matches) > 1 {
			ticket.Title = strings.TrimSpace(matches[1])
		}
	}

	// Parse phases from checkboxes
	ticket.Phases = parsePhases(content)

	return ticket, nil
}

var phaseRegex = regexp.MustCompile(`- \[([ xX])\] (.+)`)
var titleRegex = regexp.MustCompile(`(?m)^# (.+)$`)

func parsePhases(content string) []domain.Phase {
	matches := phaseRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	phases := make([]domain.Phase, 0, len(matches))
	for _, match := range matches {
		phases = append(phases, domain.Phase{
			Name:      strings.TrimSpace(match[2]),
			Completed: match[1] != " ",
		})
	}
	return phases
}

// ToWorkItem converts a Ticket to a domain.WorkItem.
func (t *Ticket) ToWorkItem() *domain.WorkItem {
	return &domain.WorkItem{
		ID:         t.ID,
		Title:      t.Title,
		Status:     t.Status,
		Phases:     t.Phases,
		RawContent: t.RawContent,
	}
}
