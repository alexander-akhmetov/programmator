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

type Client struct {
	ticketsDir string
}

func NewClient() *Client {
	dir := os.Getenv("TICKETS_DIR")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".tickets")
	}
	return &Client{ticketsDir: dir}
}

func (c *Client) Get(id string) (*Ticket, error) {
	out, err := exec.Command("ticket", "show", id).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket %s: %w", id, err)
	}

	return parseTicket(id, string(out))
}

func (c *Client) UpdatePhase(id string, phaseName string) error {
	_, err := exec.Command("ticket", "check", id, phaseName).Output()
	return err
}

func (c *Client) AddNote(id string, note string) error {
	_, err := exec.Command("ticket", "add-note", id, note).Output()
	return err
}

func (c *Client) SetStatus(id string, status string) error {
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

	// Parse phases from checkboxes
	ticket.Phases = parsePhases(content)

	return ticket, nil
}

var phaseRegex = regexp.MustCompile(`- \[([ xX])\] (.+)`)

func parsePhases(content string) []Phase {
	var phases []Phase
	matches := phaseRegex.FindAllStringSubmatch(content, -1)
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
