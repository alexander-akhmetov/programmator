// Package ticket provides a client for interacting with the ticket CLI.
package ticket

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/protocol"
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
		dir = filepath.Join(os.Getenv("HOME"), ".tickets")
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

	// Find and check the matching phase
	lines := strings.Split(string(content), "\n")
	found := false
	normalizedPhase := normalizePhase(phaseName)

	// First, try checkbox-style phases
	for i, line := range lines {
		if match := phaseRegex.FindStringSubmatch(line); match != nil {
			existingPhase := normalizePhase(match[2])
			if existingPhase == normalizedPhase || strings.Contains(existingPhase, normalizedPhase) || strings.Contains(normalizedPhase, existingPhase) {
				if match[1] != " " {
					return nil // already completed — idempotent
				}
				lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
				found = true
				break
			}
		}
	}

	// If not found in checkboxes, try heading-based phases
	if !found {
		for i, line := range lines {
			if match := headingPhaseRegex.FindStringSubmatch(line); match != nil {
				checkbox := match[5]
				prefix := match[1]
				number := match[2]
				separator := match[3]
				description := match[4]

				var fullName string
				if separator != "" {
					fullName = fmt.Sprintf("%s %s%s %s", prefix, number, separator, description)
				} else {
					fullName = fmt.Sprintf("%s %s %s", prefix, number, description)
				}
				existingPhase := normalizePhase(fullName)

				if existingPhase == normalizedPhase || strings.Contains(existingPhase, normalizedPhase) || strings.Contains(normalizedPhase, existingPhase) {
					if checkbox == "x" || checkbox == "X" {
						return nil // already completed — idempotent
					}
					if checkbox == " " {
						lines[i] = strings.Replace(line, "[ ]", "[x]", 1)
					} else {
						lines[i] = line + " [x]"
					}
					found = true
					break
				}
			}
		}
	}

	// Tier 3: numbered heading phases (e.g., "### 1. Config")
	if !found {
		for i, line := range lines {
			if match := numberedHeadingRegex.FindStringSubmatch(line); match != nil {
				headingPhase := fmt.Sprintf("%s. %s", match[2], strings.TrimSpace(match[3]))
				existingPhase := normalizePhase(headingPhase)

				if existingPhase == normalizedPhase || strings.Contains(existingPhase, normalizedPhase) || strings.Contains(normalizedPhase, existingPhase) {
					if match[4] == "x" || match[4] == "X" {
						return nil // already completed
					}
					lines[i] = line + " [x]"
					found = true
					break
				}
			}
		}
	}

	if !found {
		return fmt.Errorf("%w: %s", ErrPhaseNotFound, phaseName)
	}

	// Write atomically: temp file + rename to avoid data loss on partial write.
	// Preserve original file permissions on the temp file.
	dir := filepath.Dir(filePath)
	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat original file: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".ticket-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write([]byte(strings.Join(lines, "\n"))); err != nil {
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
	if err := os.Rename(tmpName, filePath); err != nil {
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
var normalizeNumberPrefixRegex = regexp.MustCompile(`^\d+\.\s*`)

func normalizePhase(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	// Remove common prefixes like "Phase 1:", "Step 2:", etc.
	s = normalizePrefixRegex.ReplaceAllString(s, "")
	// Remove numbered prefixes like "1.", "10." (Tier 3 numbered headings)
	s = normalizeNumberPrefixRegex.ReplaceAllString(s, "")
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
var headingPhaseRegex = regexp.MustCompile(`(?m)^## (Step|Phase) (\d+)([:.])?\s*(.+?)(?:\s*\[([xX ])\])?$`)
var numberedHeadingRegex = regexp.MustCompile(`(?m)^(#{1,6}) (\d+)\.\s+(.+?)(?:\s+\[([xX ])\])?$`)

func parsePhases(content string) []domain.Phase {
	var phases []domain.Phase

	// First, try to parse checkbox-style phases (existing behavior)
	checkboxMatches := phaseRegex.FindAllStringSubmatch(content, -1)
	for _, match := range checkboxMatches {
		phases = append(phases, domain.Phase{
			Name:      strings.TrimSpace(match[2]),
			Completed: match[1] != " ",
		})
	}

	// If we found checkbox phases, use those (backward compatibility)
	if len(phases) > 0 {
		return phases
	}

	// Otherwise, try heading-based phases (## Step N: / ## Phase N:)
	headingMatches := headingPhaseRegex.FindAllStringSubmatch(content, -1)
	for _, match := range headingMatches {
		prefix := match[1]      // "Step" or "Phase"
		number := match[2]      // the number
		separator := match[3]   // separator (: or . or empty)
		description := match[4] // description after separator
		checkbox := match[5]    // optional [x] or [ ] at the end

		// Build the phase name, preserving the original separator
		var name string
		if separator != "" {
			name = fmt.Sprintf("%s %s%s %s", prefix, number, separator, description)
		} else {
			name = fmt.Sprintf("%s %s %s", prefix, number, description)
		}

		// Determine if completed (checkbox [x] or [X] at end of line)
		completed := checkbox == "x" || checkbox == "X"

		phases = append(phases, domain.Phase{
			Name:      strings.TrimSpace(name),
			Completed: completed,
		})
	}

	if len(phases) > 0 {
		return phases
	}

	// Tier 3: sequential numbered headings
	return parseNumberedHeadingPhases(content)
}

func parseNumberedHeadingPhases(content string) []domain.Phase {
	matches := numberedHeadingRegex.FindAllStringSubmatch(content, -1)
	if len(matches) < 2 {
		return nil
	}

	// Group matches by heading level
	type headingMatch struct {
		number    int
		name      string
		completed bool
	}

	byLevel := make(map[int][]headingMatch)
	for _, m := range matches {
		level := len(m[1])
		num, _ := strconv.Atoi(m[2])
		completed := m[4] == "x" || m[4] == "X"
		byLevel[level] = append(byLevel[level], headingMatch{
			number:    num,
			name:      fmt.Sprintf("%s. %s", m[2], strings.TrimSpace(m[3])),
			completed: completed,
		})
	}

	// Try levels in ascending order
	for level := 1; level <= 6; level++ {
		group, ok := byLevel[level]
		if !ok || len(group) < 2 {
			continue
		}

		// Must start at 1
		if group[0].number != 1 {
			continue
		}

		// Find longest sequential run starting at 1
		sequential := 1
		for i := 1; i < len(group); i++ {
			if group[i].number != group[i-1].number+1 {
				break
			}
			sequential++
		}

		if sequential < 2 {
			continue
		}

		var phases []domain.Phase
		for _, h := range group[:sequential] {
			phases = append(phases, domain.Phase{
				Name:      h.name,
				Completed: h.completed,
			})
		}
		return phases
	}

	return nil
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
