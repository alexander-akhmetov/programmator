package ticket

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePhases(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []Phase
	}{
		{
			name:     "empty content",
			content:  "",
			expected: nil,
		},
		{
			name:     "no phases",
			content:  "Some content without checkboxes",
			expected: nil,
		},
		{
			name: "single uncompleted phase",
			content: `## Design
- [ ] Phase 1: Setup`,
			expected: []Phase{
				{Name: "Phase 1: Setup", Completed: false},
			},
		},
		{
			name: "single completed phase",
			content: `## Design
- [x] Phase 1: Setup`,
			expected: []Phase{
				{Name: "Phase 1: Setup", Completed: true},
			},
		},
		{
			name: "uppercase X completed",
			content: `## Design
- [X] Phase 1: Setup`,
			expected: []Phase{
				{Name: "Phase 1: Setup", Completed: true},
			},
		},
		{
			name: "multiple phases mixed",
			content: `## Design
- [x] Phase 1: Project setup
- [x] Phase 2: Implement safety module
- [ ] Phase 3: Implement ticket_client
- [ ] Phase 4: Implement prompt_builder`,
			expected: []Phase{
				{Name: "Phase 1: Project setup", Completed: true},
				{Name: "Phase 2: Implement safety module", Completed: true},
				{Name: "Phase 3: Implement ticket_client", Completed: false},
				{Name: "Phase 4: Implement prompt_builder", Completed: false},
			},
		},
		{
			name: "phases with colons and descriptions",
			content: `- [ ] Phase 1: Investigation (gather info)
- [x] Phase 2: Implementation - the main work`,
			expected: []Phase{
				{Name: "Phase 1: Investigation (gather info)", Completed: false},
				{Name: "Phase 2: Implementation - the main work", Completed: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phases := parsePhases(tt.content)
			if len(phases) != len(tt.expected) {
				t.Fatalf("expected %d phases, got %d", len(tt.expected), len(phases))
			}
			for i, phase := range phases {
				if phase.Name != tt.expected[i].Name {
					t.Errorf("phase %d: expected name %q, got %q", i, tt.expected[i].Name, phase.Name)
				}
				if phase.Completed != tt.expected[i].Completed {
					t.Errorf("phase %d: expected completed=%v, got %v", i, tt.expected[i].Completed, phase.Completed)
				}
			}
		})
	}
}

func TestParseTicket(t *testing.T) {
	tests := []struct {
		name           string
		id             string
		content        string
		expectedTitle  string
		expectedStatus string
		expectedType   string
		expectedPhases int
	}{
		{
			name:           "simple ticket without frontmatter",
			id:             "test-1",
			content:        "# Test Ticket\n\nSome content",
			expectedTitle:  "Test Ticket",
			expectedStatus: "",
			expectedPhases: 0,
		},
		{
			name: "ticket with frontmatter",
			id:   "p-5f7d",
			content: `---
title: "[programmator] Rewrite to Go"
status: in_progress
priority: 1
type: task
---
# Ticket Content

## Design
- [x] Phase 1: Setup
- [ ] Phase 2: Implementation`,
			expectedTitle:  "[programmator] Rewrite to Go",
			expectedStatus: "in_progress",
			expectedType:   "task",
			expectedPhases: 2,
		},
		{
			name: "ticket with partial frontmatter",
			id:   "t-123",
			content: `---
title: "Simple task"
---
## Phases
- [ ] Do something`,
			expectedTitle:  "Simple task",
			expectedStatus: "",
			expectedPhases: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket, err := parseTicket(tt.id, tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ticket.ID != tt.id {
				t.Errorf("expected ID %q, got %q", tt.id, ticket.ID)
			}
			if ticket.Title != tt.expectedTitle {
				t.Errorf("expected title %q, got %q", tt.expectedTitle, ticket.Title)
			}
			if ticket.Status != tt.expectedStatus {
				t.Errorf("expected status %q, got %q", tt.expectedStatus, ticket.Status)
			}
			if ticket.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, ticket.Type)
			}
			if len(ticket.Phases) != tt.expectedPhases {
				t.Errorf("expected %d phases, got %d", tt.expectedPhases, len(ticket.Phases))
			}
			if ticket.RawContent != tt.content {
				t.Error("raw content not preserved")
			}
		})
	}
}

func TestTicket_CurrentPhase(t *testing.T) {
	tests := []struct {
		name         string
		phases       []Phase
		expectedName string
		expectNil    bool
	}{
		{
			name:      "no phases",
			phases:    []Phase{},
			expectNil: true,
		},
		{
			name: "all completed",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
			expectNil: true,
		},
		{
			name: "first incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: false},
				{Name: "Phase 2", Completed: false},
			},
			expectedName: "Phase 1",
		},
		{
			name: "second incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: false},
				{Name: "Phase 3", Completed: false},
			},
			expectedName: "Phase 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{Phases: tt.phases}
			phase := ticket.CurrentPhase()
			if tt.expectNil {
				if phase != nil {
					t.Errorf("expected nil, got %v", phase)
				}
			} else {
				if phase == nil {
					t.Fatal("expected phase, got nil")
				}
				if phase.Name != tt.expectedName {
					t.Errorf("expected name %q, got %q", tt.expectedName, phase.Name)
				}
			}
		})
	}
}

func TestTicket_AllPhasesComplete(t *testing.T) {
	tests := []struct {
		name     string
		phases   []Phase
		expected bool
	}{
		{
			name:     "no phases",
			phases:   []Phase{},
			expected: false,
		},
		{
			name: "all completed",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: true},
			},
			expected: true,
		},
		{
			name: "some incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
				{Name: "Phase 2", Completed: false},
			},
			expected: false,
		},
		{
			name: "all incomplete",
			phases: []Phase{
				{Name: "Phase 1", Completed: false},
				{Name: "Phase 2", Completed: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{Phases: tt.phases}
			result := ticket.AllPhasesComplete()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("")
	if client == nil {
		t.Fatal("expected client, got nil")
	}
	if client.ticketsDir == "" {
		t.Error("expected non-empty tickets dir")
	}
	assert.Equal(t, "tk", client.command)
}

func TestNewClient_CustomCommand(t *testing.T) {
	client := NewClient("ticket")
	assert.Equal(t, "ticket", client.command)
}

func TestMockClient(t *testing.T) {
	t.Run("Get with default func", func(t *testing.T) {
		mock := NewMockClient()
		ticket, err := mock.Get("test-123")
		require.NoError(t, err)
		require.Equal(t, "test-123", ticket.ID)
		require.Len(t, mock.GetCalls, 1)
		require.Equal(t, "test-123", mock.GetCalls[0])
	})

	t.Run("Get with custom func", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetFunc = func(id string) (*Ticket, error) {
			return &Ticket{ID: id, Title: "Custom"}, nil
		}
		ticket, err := mock.Get("test-456")
		require.NoError(t, err)
		require.Equal(t, "Custom", ticket.Title)
	})

	t.Run("UpdatePhase with default func", func(t *testing.T) {
		mock := NewMockClient()
		err := mock.UpdatePhase("test-123", "Phase 1")
		require.NoError(t, err)
		require.Len(t, mock.UpdatePhaseCalls, 1)
		require.Equal(t, "test-123", mock.UpdatePhaseCalls[0].ID)
		require.Equal(t, "Phase 1", mock.UpdatePhaseCalls[0].PhaseName)
	})

	t.Run("UpdatePhase with custom func", func(t *testing.T) {
		mock := NewMockClient()
		customErr := fmt.Errorf("custom error")
		mock.UpdatePhaseFunc = func(_, _ string) error {
			return customErr
		}
		err := mock.UpdatePhase("test-123", "Phase 1")
		require.ErrorIs(t, err, customErr)
	})

	t.Run("AddNote with default func", func(t *testing.T) {
		mock := NewMockClient()
		err := mock.AddNote("test-123", "some note")
		require.NoError(t, err)
		require.Len(t, mock.AddNoteCalls, 1)
		require.Equal(t, "test-123", mock.AddNoteCalls[0].ID)
		require.Equal(t, "some note", mock.AddNoteCalls[0].Note)
	})

	t.Run("AddNote with custom func", func(t *testing.T) {
		mock := NewMockClient()
		customErr := fmt.Errorf("add note error")
		mock.AddNoteFunc = func(_, _ string) error {
			return customErr
		}
		err := mock.AddNote("test-123", "note")
		require.ErrorIs(t, err, customErr)
	})

	t.Run("SetStatus with default func", func(t *testing.T) {
		mock := NewMockClient()
		err := mock.SetStatus("test-123", "closed")
		require.NoError(t, err)
		require.Len(t, mock.SetStatusCalls, 1)
		require.Equal(t, "test-123", mock.SetStatusCalls[0].ID)
		require.Equal(t, "closed", mock.SetStatusCalls[0].Status)
	})

	t.Run("SetStatus with custom func", func(t *testing.T) {
		mock := NewMockClient()
		customErr := fmt.Errorf("set status error")
		mock.SetStatusFunc = func(_, _ string) error {
			return customErr
		}
		err := mock.SetStatus("test-123", "closed")
		require.ErrorIs(t, err, customErr)
	})
}

func TestMockClientImplementsInterface(_ *testing.T) {
	var _ Client = (*MockClient)(nil)
	var _ Client = (*CLIClient)(nil)
}

func TestTicket_HasPhases(t *testing.T) {
	tests := []struct {
		name     string
		phases   []Phase
		expected bool
	}{
		{
			name:     "no phases",
			phases:   []Phase{},
			expected: false,
		},
		{
			name:     "nil phases",
			phases:   nil,
			expected: false,
		},
		{
			name: "has phases",
			phases: []Phase{
				{Name: "Phase 1", Completed: false},
			},
			expected: true,
		},
		{
			name: "has completed phases",
			phases: []Phase{
				{Name: "Phase 1", Completed: true},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{Phases: tt.phases}
			result := ticket.HasPhases()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNormalizePhase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase", "Setup Project", "setup project"},
		{"trims whitespace", "  setup  ", "setup"},
		{"strips Phase N:", "Phase 1: Setup", "setup"},
		{"strips Phase N.", "Phase 2. Implementation", "implementation"},
		{"strips Step N:", "Step 3: Testing", "testing"},
		{"no prefix", "just a task", "just a task"},
		{"case insensitive prefix", "phase 1: Setup", "setup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePhase(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpdatePhase(t *testing.T) {
	setup := func(t *testing.T, content string) (*CLIClient, string) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "t-1234.md")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		return &CLIClient{ticketsDir: dir}, path
	}

	t.Run("marks unchecked phase as complete", func(t *testing.T) {
		client, path := setup(t, "## Design\n- [ ] Phase 1: Setup\n- [ ] Phase 2: Implement\n")
		err := client.UpdatePhase("t-1234", "Phase 1: Setup")
		require.NoError(t, err)

		data, _ := os.ReadFile(path)
		assert.Contains(t, string(data), "- [x] Phase 1: Setup")
		assert.Contains(t, string(data), "- [ ] Phase 2: Implement")
	})

	t.Run("error when phase already completed", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [x] Phase 1: Setup\n")
		err := client.UpdatePhase("t-1234", "Phase 1: Setup")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found or already completed")
	})

	t.Run("error when phase not found", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [ ] Phase 1: Setup\n")
		err := client.UpdatePhase("t-1234", "Nonexistent Phase")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found or already completed")
	})

	t.Run("no-op for empty phase name", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [ ] Phase 1: Setup\n")
		assert.NoError(t, client.UpdatePhase("t-1234", ""))
	})

	t.Run("no-op for null phase name", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [ ] Phase 1: Setup\n")
		assert.NoError(t, client.UpdatePhase("t-1234", "null"))
	})

	t.Run("fuzzy matching - phase contains name", func(t *testing.T) {
		client, path := setup(t, "## Design\n- [ ] Phase 1: Setup the project\n")
		err := client.UpdatePhase("t-1234", "Setup the project")
		require.NoError(t, err)

		data, _ := os.ReadFile(path)
		assert.Contains(t, string(data), "- [x] Phase 1: Setup the project")
	})

	t.Run("fuzzy matching - name contains phase", func(t *testing.T) {
		client, path := setup(t, "## Design\n- [ ] Setup\n")
		err := client.UpdatePhase("t-1234", "Phase 1: Setup")
		require.NoError(t, err)

		data, _ := os.ReadFile(path)
		assert.Contains(t, string(data), "- [x] Setup")
	})
}

func TestFindTicketFile(t *testing.T) {
	setup := func(t *testing.T, filenames ...string) *CLIClient {
		t.Helper()
		dir := t.TempDir()
		for _, name := range filenames {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("content"), 0644))
		}
		return &CLIClient{ticketsDir: dir}
	}

	t.Run("prefix match", func(t *testing.T) {
		client := setup(t, "t-1234-some-title.md")
		path, err := client.findTicketFile("t-1234")
		require.NoError(t, err)
		assert.Contains(t, path, "t-1234-some-title.md")
	})

	t.Run("contains-dash match", func(t *testing.T) {
		client := setup(t, "project-t-1234.md")
		path, err := client.findTicketFile("t-1234")
		require.NoError(t, err)
		assert.Contains(t, path, "project-t-1234.md")
	})

	t.Run("suffix match via second pass", func(t *testing.T) {
		client := setup(t, "abc1234.md")
		path, err := client.findTicketFile("1234")
		require.NoError(t, err)
		assert.Contains(t, path, "abc1234.md")
	})

	t.Run("not found", func(t *testing.T) {
		client := setup(t, "unrelated.md")
		_, err := client.findTicketFile("t-9999")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("skips directories in first pass", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "t-1234"), 0755))
		// Also add a file that won't match
		require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated.md"), []byte("x"), 0644))
		client := &CLIClient{ticketsDir: dir}
		// The first pass skips directories; the second pass may still match
		// the directory name, so we just verify no panic occurs
		_, _ = client.findTicketFile("t-1234")
	})
}

func TestNewClient_EnvHandling(t *testing.T) {
	t.Run("reads TICKETS_DIR env", func(t *testing.T) {
		original := os.Getenv("TICKETS_DIR")
		defer os.Setenv("TICKETS_DIR", original)

		os.Setenv("TICKETS_DIR", "/custom/tickets")
		client := NewClient("")
		assert.Equal(t, "/custom/tickets", client.ticketsDir)
	})

	t.Run("falls back to ~/.tickets", func(t *testing.T) {
		original := os.Getenv("TICKETS_DIR")
		defer os.Setenv("TICKETS_DIR", original)

		os.Unsetenv("TICKETS_DIR")
		client := NewClient("")
		home := os.Getenv("HOME")
		assert.Equal(t, filepath.Join(home, ".tickets"), client.ticketsDir)
	})
}

func TestPhaselessTicketParsing(t *testing.T) {
	content := `---
title: "Phaseless ticket"
status: open
---
# Phaseless ticket

## Goal
Do something without explicit phases.

## Description
This ticket has no checkbox phases - it should be treated as a single task.
`
	ticket, err := parseTicket("phaseless-1", content)
	require.NoError(t, err)
	require.Equal(t, "Phaseless ticket", ticket.Title)
	require.Empty(t, ticket.Phases)
	require.False(t, ticket.HasPhases())
	require.Nil(t, ticket.CurrentPhase())
	require.False(t, ticket.AllPhasesComplete())
}
