package ticket

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestParsePhases(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []domain.Phase
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
			expected: []domain.Phase{
				{Name: "Phase 1: Setup", Completed: false},
			},
		},
		{
			name: "single completed phase",
			content: `## Design
- [x] Phase 1: Setup`,
			expected: []domain.Phase{
				{Name: "Phase 1: Setup", Completed: true},
			},
		},
		{
			name: "uppercase X completed",
			content: `## Design
- [X] Phase 1: Setup`,
			expected: []domain.Phase{
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
			expected: []domain.Phase{
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
			expected: []domain.Phase{
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
			expectedStatus: protocol.WorkItemInProgress,
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

func TestTicket_ToWorkItem(t *testing.T) {
	ticket := &Ticket{
		ID:         "t-123",
		Title:      "Test Ticket",
		Status:     protocol.WorkItemOpen,
		Phases:     []domain.Phase{{Name: "Phase 1", Completed: true}, {Name: "Phase 2", Completed: false}},
		RawContent: "raw",
	}

	item := ticket.ToWorkItem()
	assert.Equal(t, "t-123", item.ID)
	assert.Equal(t, "Test Ticket", item.Title)
	assert.Equal(t, protocol.WorkItemOpen, item.Status)
	assert.Equal(t, "raw", item.RawContent)
	require.Len(t, item.Phases, 2)
	assert.Equal(t, "Phase 1", item.Phases[0].Name)
	assert.True(t, item.Phases[0].Completed)
	assert.Equal(t, "Phase 2", item.Phases[1].Name)
	assert.False(t, item.Phases[1].Completed)

	// Domain methods work via WorkItem
	assert.True(t, item.HasPhases())
	require.NotNil(t, item.CurrentPhase())
	assert.Equal(t, "Phase 2", item.CurrentPhase().Name)
	assert.False(t, item.AllPhasesComplete())
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
		err := mock.SetStatus("test-123", protocol.WorkItemClosed)
		require.NoError(t, err)
		require.Len(t, mock.SetStatusCalls, 1)
		require.Equal(t, "test-123", mock.SetStatusCalls[0].ID)
		require.Equal(t, protocol.WorkItemClosed, mock.SetStatusCalls[0].Status)
	})

	t.Run("SetStatus with custom func", func(t *testing.T) {
		mock := NewMockClient()
		customErr := fmt.Errorf("set status error")
		mock.SetStatusFunc = func(_, _ string) error {
			return customErr
		}
		err := mock.SetStatus("test-123", protocol.WorkItemClosed)
		require.ErrorIs(t, err, customErr)
	})
}

func TestMockClientImplementsInterface(t *testing.T) {
	require.Implements(t, (*Client)(nil), &MockClient{})
	require.Implements(t, (*Client)(nil), &CLIClient{})
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
		{"strips numbered prefix 1.", "1. Config", "config"},
		{"strips numbered prefix 10.", "10. Tests", "tests"},
		{"strips numbered prefix with space", "3.  Setup", "setup"},
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

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "- [x] Phase 1: Setup")
		assert.Contains(t, string(data), "- [ ] Phase 2: Implement")
	})

	t.Run("no-op when phase already completed", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [x] Phase 1: Setup\n")
		err := client.UpdatePhase("t-1234", "Phase 1: Setup")
		assert.NoError(t, err)
	})

	t.Run("error when phase not found", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [ ] Phase 1: Setup\n")
		err := client.UpdatePhase("t-1234", "Nonexistent Phase")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrPhaseNotFound))
	})

	t.Run("no-op for empty phase name", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [ ] Phase 1: Setup\n")
		assert.NoError(t, client.UpdatePhase("t-1234", ""))
	})

	t.Run("no-op for null phase name", func(t *testing.T) {
		client, _ := setup(t, "## Design\n- [ ] Phase 1: Setup\n")
		assert.NoError(t, client.UpdatePhase("t-1234", protocol.NullPhase))
	})

	t.Run("fuzzy matching - phase contains name", func(t *testing.T) {
		client, path := setup(t, "## Design\n- [ ] Phase 1: Setup the project\n")
		err := client.UpdatePhase("t-1234", "Setup the project")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "- [x] Phase 1: Setup the project")
	})

	t.Run("fuzzy matching - name contains phase", func(t *testing.T) {
		client, path := setup(t, "## Design\n- [ ] Setup\n")
		err := client.UpdatePhase("t-1234", "Phase 1: Setup")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "- [x] Setup")
	})
}

func TestUpdatePhase_OverlappingNames(t *testing.T) {
	dir := t.TempDir()
	content := "## Design\n- [ ] Setup\n- [ ] Setup Tests\n- [ ] Setup Integration Tests\n"
	path := filepath.Join(dir, "t-1234.md")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	client := &CLIClient{ticketsDir: dir}

	// "Setup" should match the first phase (exact match after normalization)
	err := client.UpdatePhase("t-1234", "Setup")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "- [x] Setup")
	assert.Contains(t, string(data), "- [ ] Setup Tests")
	assert.Contains(t, string(data), "- [ ] Setup Integration Tests")
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

	t.Run("finds by id", func(t *testing.T) {
		client := setup(t, "t-1234.md")
		path, err := client.findTicketFile("t-1234")
		require.NoError(t, err)
		assert.Contains(t, path, "t-1234.md")
	})

	t.Run("exact match among similar prefixes", func(t *testing.T) {
		client := setup(t, "pro-0frj.md", "pro-1995.md", "pro-l54x.md", "pro-twlj.md")
		path, err := client.findTicketFile("pro-l54x")
		require.NoError(t, err)
		assert.Contains(t, path, "pro-l54x.md")
	})

	t.Run("not found", func(t *testing.T) {
		client := setup(t, "unrelated.md")
		_, err := client.findTicketFile("t-9999")
		assert.Error(t, err)
		assert.True(t, errors.Is(err, ErrTicketNotFound))
	})
}

func TestFindTicketFile_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	client := &CLIClient{ticketsDir: dir}

	tests := []struct {
		name string
		id   string
	}{
		{"parent traversal", "../../../etc/passwd"},
		{"dot-dot in middle", "foo/../bar"},
		{"absolute path chars", "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.findTicketFile(tt.id)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrTicketNotFound)
		})
	}
}

func TestValidateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid simple", "t-1234", false},
		{"valid with dots", "pro-l54x.v2", false},
		{"valid alphanumeric", "abc123", false},
		{"empty", "", true},
		{"path traversal", "../etc/passwd", true},
		{"semicolon injection", "id;rm -rf", true},
		{"starts with dash", "-flag", true},
		{"spaces", "id with spaces", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrTicketNotFound)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSetStatus_InvalidStatus(t *testing.T) {
	client := &CLIClient{ticketsDir: t.TempDir(), command: "tk"}
	err := client.SetStatus("t-123", "invalid_status")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid status")
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
	item := ticket.ToWorkItem()
	require.False(t, item.HasPhases())
	require.Nil(t, item.CurrentPhase())
	require.False(t, item.AllPhasesComplete())
}

func TestParsePhases_HeadingBased(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []domain.Phase
	}{
		{
			name: "single step with colon",
			content: `# Plan
## Step 1: Add new agent prompts
Some content here`,
			expected: []domain.Phase{
				{Name: "Step 1: Add new agent prompts", Completed: false},
			},
		},
		{
			name: "single step with period",
			content: `# Plan
## Step 1. Add new agent prompts
Some content here`,
			expected: []domain.Phase{
				{Name: "Step 1. Add new agent prompts", Completed: false},
			},
		},
		{
			name: "multiple steps",
			content: `# Plan
## Step 1: Add 3 New Agent Prompts
Content for step 1
## Step 2: Add Phase Config Model
Content for step 2
## Step 3: Add Severity Filtering to RunResult
Content for step 3`,
			expected: []domain.Phase{
				{Name: "Step 1: Add 3 New Agent Prompts", Completed: false},
				{Name: "Step 2: Add Phase Config Model", Completed: false},
				{Name: "Step 3: Add Severity Filtering to RunResult", Completed: false},
			},
		},
		{
			name: "steps with checkboxes at end",
			content: `# Plan
## Step 1: Investigation [x]
## Step 2: Implementation [ ]
## Step 3: Testing [X]`,
			expected: []domain.Phase{
				{Name: "Step 1: Investigation", Completed: true},
				{Name: "Step 2: Implementation", Completed: false},
				{Name: "Step 3: Testing", Completed: true},
			},
		},
		{
			name: "phase keyword instead of step",
			content: `# Plan
## Phase 1: Setup
## Phase 2: Implementation`,
			expected: []domain.Phase{
				{Name: "Phase 1: Setup", Completed: false},
				{Name: "Phase 2: Implementation", Completed: false},
			},
		},
		{
			name: "mixed with other headings (should only match Step/Phase)",
			content: `# Title
## Step 1: First task
## Notes
Some notes here
## Step 2: Second task
## References
More content`,
			expected: []domain.Phase{
				{Name: "Step 1: First task", Completed: false},
				{Name: "Step 2: Second task", Completed: false},
			},
		},
		{
			name: "checkbox phases take precedence over heading phases",
			content: `# Plan
## Design
- [ ] Phase 1: Investigation
- [ ] Phase 2: Implementation
## Step 1: Add new prompts
## Step 2: Update config`,
			expected: []domain.Phase{
				{Name: "Phase 1: Investigation", Completed: false},
				{Name: "Phase 2: Implementation", Completed: false},
			},
		},
		{
			name: "no phases when no matching patterns",
			content: `# Plan
## Introduction
Some text
## Conclusion
More text`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phases := parsePhases(tt.content)
			require.Equal(t, len(tt.expected), len(phases), "expected %d phases, got %d", len(tt.expected), len(phases))
			for i, phase := range phases {
				assert.Equal(t, tt.expected[i].Name, phase.Name, "phase %d name mismatch", i)
				assert.Equal(t, tt.expected[i].Completed, phase.Completed, "phase %d completed status mismatch", i)
			}
		})
	}
}

func TestParsePhases_NumberedHeadings(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []domain.Phase
	}{
		{
			name: "basic three numbered headings",
			content: `# Plan
### 1. Config
### 2. Parser
### 3. Tests`,
			expected: []domain.Phase{
				{Name: "1. Config", Completed: false},
				{Name: "2. Parser", Completed: false},
				{Name: "3. Tests", Completed: false},
			},
		},
		{
			name: "different heading level (h2)",
			content: `# Plan
## 1. Setup
## 2. Implement`,
			expected: []domain.Phase{
				{Name: "1. Setup", Completed: false},
				{Name: "2. Implement", Completed: false},
			},
		},
		{
			name:     "below minimum - single heading returns nil",
			content:  `### 1. Only one`,
			expected: nil,
		},
		{
			name: "gap in numbering - no sequential run from 1",
			content: `### 1. A
### 3. C`,
			expected: nil,
		},
		{
			name: "out of order headings return nil",
			content: `### 2. Second
### 1. First`,
			expected: nil,
		},
		{
			name: "partial sequential - truncates at gap",
			content: `### 1. A
### 2. B
### 5. C`,
			expected: []domain.Phase{
				{Name: "1. A", Completed: false},
				{Name: "2. B", Completed: false},
			},
		},
		{
			name: "checkbox phases take precedence over numbered headings",
			content: `# Plan
- [ ] Setup
- [ ] Implement
### 1. Config
### 2. Parser`,
			expected: []domain.Phase{
				{Name: "Setup", Completed: false},
				{Name: "Implement", Completed: false},
			},
		},
		{
			name: "heading phases take precedence over numbered headings",
			content: `# Plan
## Step 1: Investigation
## Step 2: Implementation
### 1. Config
### 2. Parser`,
			expected: []domain.Phase{
				{Name: "Step 1: Investigation", Completed: false},
				{Name: "Step 2: Implementation", Completed: false},
			},
		},
		{
			name: "starts at 0 returns nil",
			content: `### 0. Intro
### 1. Config
### 2. Parser`,
			expected: nil,
		},
		{
			name: "headings with descriptions",
			content: `# Plan
### 1. Config (YAML parsing)
### 2. Node types (AST nodes)
### 3. Tests (unit + integration)`,
			expected: []domain.Phase{
				{Name: "1. Config (YAML parsing)", Completed: false},
				{Name: "2. Node types (AST nodes)", Completed: false},
				{Name: "3. Tests (unit + integration)", Completed: false},
			},
		},
		{
			name: "completed [x] checkbox parsed correctly",
			content: `# Plan
### 1. Done [x]
### 2. Todo`,
			expected: []domain.Phase{
				{Name: "1. Done", Completed: true},
				{Name: "2. Todo", Completed: false},
			},
		},
		{
			name: "completed [x] checkbox parsed without space",
			content: `# Plan
### 1. Done[x]
### 2. Todo`,
			expected: []domain.Phase{
				{Name: "1. Done", Completed: true},
				{Name: "2. Todo", Completed: false},
			},
		},
		{
			name: "mixed levels - picks lowest sequential level",
			content: `## 1. High level A
## 2. High level B
### 1. Sub A
### 2. Sub B
### 3. Sub C`,
			expected: []domain.Phase{
				{Name: "1. High level A", Completed: false},
				{Name: "2. High level B", Completed: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phases := parsePhases(tt.content)
			require.Equal(t, len(tt.expected), len(phases), "expected %d phases, got %d", len(tt.expected), len(phases))
			for i, phase := range phases {
				assert.Equal(t, tt.expected[i].Name, phase.Name, "phase %d name mismatch", i)
				assert.Equal(t, tt.expected[i].Completed, phase.Completed, "phase %d completed status mismatch", i)
			}
		})
	}
}

func TestUpdatePhase_HeadingBased(t *testing.T) {
	setup := func(t *testing.T, content string) (*CLIClient, string) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "t-1234.md")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		return &CLIClient{ticketsDir: dir}, path
	}

	t.Run("marks heading phase as complete by adding checkbox", func(t *testing.T) {
		content := `# Plan
## Step 1: Add new agent prompts
## Step 2: Update config
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "Step 1: Add new agent prompts")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "## Step 1: Add new agent prompts [x]")
		assert.Contains(t, string(data), "## Step 2: Update config")
	})

	t.Run("updates existing checkbox marker on heading phase", func(t *testing.T) {
		content := `# Plan
## Step 1: Investigation [ ]
## Step 2: Implementation [ ]
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "Step 1: Investigation")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "## Step 1: Investigation [x]")
		assert.Contains(t, string(data), "## Step 2: Implementation [ ]")
	})

	t.Run("no-op when heading phase already completed", func(t *testing.T) {
		content := `# Plan
## Step 1: Investigation [x]
`
		client, _ := setup(t, content)
		err := client.UpdatePhase("t-1234", "Step 1: Investigation")
		assert.NoError(t, err)
	})

	t.Run("fuzzy matching for heading phases", func(t *testing.T) {
		content := `# Plan
## Step 1: Add new agent prompts
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "Add new agent prompts")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "## Step 1: Add new agent prompts [x]")
	})

	t.Run("prefers checkbox phases over heading phases", func(t *testing.T) {
		content := `# Plan
- [ ] Phase 1: Investigation
## Step 1: Add new prompts
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "Phase 1: Investigation")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		// Should update the checkbox, not the heading
		assert.Contains(t, string(data), "- [x] Phase 1: Investigation")
		assert.Contains(t, string(data), "## Step 1: Add new prompts")
		assert.NotContains(t, string(data), "## Step 1: Add new prompts [x]")
	})
}

func TestUpdatePhase_NumberedHeadings(t *testing.T) {
	setup := func(t *testing.T, content string) (*CLIClient, string) {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "t-1234.md")
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
		return &CLIClient{ticketsDir: dir}, path
	}

	t.Run("marks numbered heading as complete by appending [x]", func(t *testing.T) {
		content := `# Plan
### 1. Config
### 2. Parser
### 3. Tests
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "1. Config")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "### 1. Config [x]")
		assert.Contains(t, string(data), "### 2. Parser\n")
		assert.Contains(t, string(data), "### 3. Tests\n")
	})

	t.Run("idempotent when already marked [x]", func(t *testing.T) {
		content := `# Plan
### 1. Config [x]
### 2. Parser
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "1. Config")
		assert.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("idempotent when already marked [X]", func(t *testing.T) {
		content := `# Plan
### 1. Config [X]
### 2. Parser
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "1. Config")
		assert.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("updates existing [ ] checkbox", func(t *testing.T) {
		content := `# Plan
### 1. Config [ ]
### 2. Parser
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "1. Config")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "### 1. Config [x]")
		assert.Contains(t, string(data), "### 2. Parser")
		assert.NotContains(t, string(data), "### 1. Config [ ] [x]")
	})

	t.Run("fuzzy matching works with normalized names", func(t *testing.T) {
		content := `# Plan
### 1. Config setup
### 2. Parser implementation
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "Config setup")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "### 1. Config setup [x]")
	})

	t.Run("error when phase not found", func(t *testing.T) {
		content := `# Plan
### 1. Config
### 2. Parser
`
		client, _ := setup(t, content)
		err := client.UpdatePhase("t-1234", "Nonexistent Phase")
		assert.ErrorIs(t, err, ErrPhaseNotFound)
	})

	t.Run("prefers checkbox over numbered heading", func(t *testing.T) {
		content := `# Plan
- [ ] Config setup
### 1. Config setup
### 2. Parser
`
		client, path := setup(t, content)
		err := client.UpdatePhase("t-1234", "Config setup")
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "- [x] Config setup")
		assert.NotContains(t, string(data), "### 1. Config setup [x]")
	})
}

func TestParseTicket_HeadingBasedPhases(t *testing.T) {
	content := `---
id: pro-wq1b
status: closed
---
# [programmator] Plan: Multi-Phase Review System

## Step 1: Add 3 New Agent Prompts

Create new embedded prompt files.

## Step 2: Add Phase Config Model

Add Phase struct alongside existing Pass.

## Step 3: Add Severity Filtering to RunResult

Add FilterBySeverity method.
`
	ticket, err := parseTicket("pro-wq1b", content)
	require.NoError(t, err)
	require.Equal(t, "[programmator] Plan: Multi-Phase Review System", ticket.Title)
	require.Len(t, ticket.Phases, 3)
	item := ticket.ToWorkItem()
	require.True(t, item.HasPhases())

	assert.Equal(t, "Step 1: Add 3 New Agent Prompts", ticket.Phases[0].Name)
	assert.False(t, ticket.Phases[0].Completed)
	assert.Equal(t, "Step 2: Add Phase Config Model", ticket.Phases[1].Name)
	assert.False(t, ticket.Phases[1].Completed)
	assert.Equal(t, "Step 3: Add Severity Filtering to RunResult", ticket.Phases[2].Name)
	assert.False(t, ticket.Phases[2].Completed)

	currentPhase := item.CurrentPhase()
	require.NotNil(t, currentPhase)
	assert.Equal(t, "Step 1: Add 3 New Agent Prompts", currentPhase.Name)
}
