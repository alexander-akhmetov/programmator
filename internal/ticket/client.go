// Package ticket provides a client for interacting with the ticket CLI.
package ticket

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Phase struct {
	Name      string
	Completed bool
}

type Ticket struct {
	ID          string
	Title       string
	Status      string
	Priority    int
	Type        string
	Description string
	Phases      []Phase
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
	out, err := exec.Command(c.command, "show", id).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket %s: %w", id, err)
	}

	return parseTicket(id, string(out))
}

func (c *CLIClient) UpdatePhase(id string, phaseName string) error {
	// For phaseless tickets, there's nothing to update
	if phaseName == "" || phaseName == "null" {
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
			if match[1] == " " { // unchecked
				existingPhase := normalizePhase(match[2])
				if existingPhase == normalizedPhase || strings.Contains(existingPhase, normalizedPhase) || strings.Contains(normalizedPhase, existingPhase) {
					lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
					found = true
					break
				}
			}
		}
	}

	// If not found in checkboxes, try heading-based phases
	if !found {
		for i, line := range lines {
			if match := headingPhaseRegex.FindStringSubmatch(line); match != nil {
				// Check if it has a checkbox marker at the end
				checkbox := match[5]
				if checkbox == " " || checkbox == "" { // unchecked or no checkbox
					prefix := match[1]
					number := match[2]
					separator := match[3]
					description := match[4]

					// Build the full name with the original separator
					var fullName string
					if separator != "" {
						fullName = fmt.Sprintf("%s %s%s %s", prefix, number, separator, description)
					} else {
						fullName = fmt.Sprintf("%s %s %s", prefix, number, description)
					}
					existingPhase := normalizePhase(fullName)

					if existingPhase == normalizedPhase || strings.Contains(existingPhase, normalizedPhase) || strings.Contains(normalizedPhase, existingPhase) {
						// Add or update checkbox marker at end of line
						if checkbox == " " {
							// Replace [ ] with [x]
							lines[i] = strings.Replace(line, "[ ]", "[x]", 1)
						} else {
							// Add [x] at the end
							lines[i] = line + " [x]"
						}
						found = true
						break
					}
				}
			}
		}
	}

	if !found {
		return fmt.Errorf("phase not found or already completed: %s", phaseName)
	}

	return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644)
}

func (c *CLIClient) findTicketFile(id string) (string, error) {
	entries, err := os.ReadDir(c.ticketsDir)
	if err != nil {
		return "", fmt.Errorf("read tickets dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, id) || strings.Contains(name, "-"+id) || strings.HasPrefix(name, id[:min(len(id), 4)]) {
			return filepath.Join(c.ticketsDir, name), nil
		}
	}

	// Try with common prefixes
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".md")
		if strings.HasSuffix(name, id) || strings.Contains(name, id) {
			return filepath.Join(c.ticketsDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("ticket file not found for id: %s", id)
}

func normalizePhase(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	// Remove common prefixes like "Phase 1:", "Step 2:", etc.
	s = regexp.MustCompile(`^(phase|step)\s*\d+[:.]\s*`).ReplaceAllString(s, "")
	return s
}

func (c *CLIClient) AddNote(id string, note string) error {
	_, err := exec.Command(c.command, "add-note", id, note).Output()
	return err
}

func (c *CLIClient) SetStatus(id string, status string) error {
	_, err := exec.Command(c.command, "set-status", id, status).Output()
	return err
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

func parsePhases(content string) []Phase {
	var phases []Phase

	// First, try to parse checkbox-style phases (existing behavior)
	checkboxMatches := phaseRegex.FindAllStringSubmatch(content, -1)
	for _, match := range checkboxMatches {
		phases = append(phases, Phase{
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

		phases = append(phases, Phase{
			Name:      strings.TrimSpace(name),
			Completed: completed,
		})
	}

	return phases
}

func (t *Ticket) CurrentPhase() *Phase {
	for i := range t.Phases {
		if !t.Phases[i].Completed {
			return &t.Phases[i]
		}
	}
	return nil
}

func (t *Ticket) AllPhasesComplete() bool {
	for _, p := range t.Phases {
		if !p.Completed {
			return false
		}
	}
	return len(t.Phases) > 0
}

// HasPhases returns true if the ticket has any phases defined.
func (t *Ticket) HasPhases() bool {
	return len(t.Phases) > 0
}
