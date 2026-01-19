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
}

var _ Client = (*CLIClient)(nil)

func NewClient() *CLIClient {
	dir := os.Getenv("TICKETS_DIR")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".tickets")
	}
	return &CLIClient{ticketsDir: dir}
}

func (c *CLIClient) Get(id string) (*Ticket, error) {
	out, err := exec.Command("ticket", "show", id).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket %s: %w", id, err)
	}

	return parseTicket(id, string(out))
}

func (c *CLIClient) UpdatePhase(id string, phaseName string) error {
	_, err := exec.Command("ticket", "check", id, phaseName).Output()
	return err
}

func (c *CLIClient) AddNote(id string, note string) error {
	_, err := exec.Command("ticket", "add-note", id, note).Output()
	return err
}

func (c *CLIClient) SetStatus(id string, status string) error {
	_, err := exec.Command("ticket", "set-status", id, status).Output()
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

func parsePhases(content string) []Phase {
	matches := phaseRegex.FindAllStringSubmatch(content, -1)
	phases := make([]Phase, 0, len(matches))
	for _, match := range matches {
		phases = append(phases, Phase{
			Name:      strings.TrimSpace(match[2]),
			Completed: match[1] != " ",
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
