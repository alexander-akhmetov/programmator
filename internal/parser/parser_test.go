package parser

import (
	"testing"

	"github.com/alexander-akhmetov/programmator/internal/protocol"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantNil     bool
		wantErr     bool
		wantPhase   string
		wantStatus  Status
		wantFiles   []string
		wantSummary string
		wantError   string
	}{
		// NOTE: See TestParseCommitMade for commit_made field tests
		{
			name: "basic CONTINUE status",
			output: `Some output text here

` + "```" + `
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1: Setup"
  status: CONTINUE
  files_changed:
    - main.go
    - util.go
  summary: "Implemented basic setup"
` + "```",
			wantNil:     false,
			wantPhase:   "Phase 1: Setup",
			wantStatus:  protocol.StatusContinue,
			wantFiles:   []string{"main.go", "util.go"},
			wantSummary: "Implemented basic setup",
		},
		{
			name: "DONE status",
			output: `Task completed

PROGRAMMATOR_STATUS:
  phase_completed: "Phase 5: Final"
  status: DONE
  files_changed:
    - README.md
  summary: "All phases complete"
` + "```",
			wantNil:     false,
			wantPhase:   "Phase 5: Final",
			wantStatus:  protocol.StatusDone,
			wantFiles:   []string{"README.md"},
			wantSummary: "All phases complete",
		},
		{
			name: "BLOCKED status with error",
			output: `Cannot proceed

PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Attempted to connect to database"
  error: "Database credentials not configured"
` + "```",
			wantNil:     false,
			wantPhase:   "",
			wantStatus:  protocol.StatusBlocked,
			wantFiles:   []string{},
			wantSummary: "Attempted to connect to database",
			wantError:   "Database credentials not configured",
		},
		{
			name: "status block without trailing backticks",
			output: `Output text

PROGRAMMATOR_STATUS:
  phase_completed: "Phase 2"
  status: CONTINUE
  files_changed: []
  summary: "Did some work"`,
			wantNil:     false,
			wantPhase:   "Phase 2",
			wantStatus:  protocol.StatusContinue,
			wantFiles:   []string{},
			wantSummary: "Did some work",
		},
		{
			name:    "no status block",
			output:  "Some output without status block",
			wantNil: true,
		},
		{
			name:    "empty output",
			output:  "",
			wantNil: true,
		},
		{
			name: "status block with extra whitespace",
			output: `PROGRAMMATOR_STATUS:
  phase_completed:    "Phase 1"
  status:     CONTINUE
  files_changed:
    - file1.go
  summary:   "Done"
` + "```" + `
`,
			wantNil:     false,
			wantPhase:   "Phase 1",
			wantStatus:  protocol.StatusContinue,
			wantFiles:   []string{"file1.go"},
			wantSummary: "Done",
		},
		{
			name: "null phase_completed as string",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed: []
  summary: "In progress"
`,
			wantNil:     false,
			wantPhase:   "",
			wantStatus:  protocol.StatusContinue,
			wantFiles:   []string{},
			wantSummary: "In progress",
		},
		{
			name: "empty files_changed array",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: CONTINUE
  files_changed: []
  summary: "Researched"
`,
			wantNil:     false,
			wantPhase:   "Phase 1",
			wantStatus:  protocol.StatusContinue,
			wantFiles:   []string{},
			wantSummary: "Researched",
		},
		{
			name: "inline files_changed array",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: CONTINUE
  files_changed: [main.go, util.go, test.go]
  summary: "Multiple files"
`,
			wantNil:     false,
			wantPhase:   "Phase 1",
			wantStatus:  protocol.StatusContinue,
			wantFiles:   []string{"main.go", "util.go", "test.go"},
			wantSummary: "Multiple files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.output)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse() expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Parse() unexpected error: %v", err)
				return
			}

			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse() expected nil but got %+v", got)
				}
				return
			}

			if got == nil {
				t.Errorf("Parse() expected non-nil result")
				return
			}

			if got.PhaseCompleted != tt.wantPhase {
				t.Errorf("PhaseCompleted = %q, want %q", got.PhaseCompleted, tt.wantPhase)
			}

			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}

			if got.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", got.Summary, tt.wantSummary)
			}

			if got.Error != tt.wantError {
				t.Errorf("Error = %q, want %q", got.Error, tt.wantError)
			}

			if len(got.FilesChanged) != len(tt.wantFiles) {
				t.Errorf("FilesChanged length = %d, want %d", len(got.FilesChanged), len(tt.wantFiles))
			} else {
				for i, f := range got.FilesChanged {
					if f != tt.wantFiles[i] {
						t.Errorf("FilesChanged[%d] = %q, want %q", i, f, tt.wantFiles[i])
					}
				}
			}
		})
	}
}

func TestParseCommitMade(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		wantCommitMade bool
	}{
		{
			name: "commit_made true",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - main.go
  summary: "Fixed issue"
  commit_made: true
`,
			wantCommitMade: true,
		},
		{
			name: "commit_made false",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - main.go
  summary: "Fixed issue"
  commit_made: false
`,
			wantCommitMade: false,
		},
		{
			name: "commit_made omitted",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: null
  status: CONTINUE
  files_changed:
    - main.go
  summary: "Fixed issue"
`,
			wantCommitMade: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.output)
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("Parse() returned nil")
			}
			if got.CommitMade != tt.wantCommitMade {
				t.Errorf("CommitMade = %v, want %v", got.CommitMade, tt.wantCommitMade)
			}
		})
	}
}

func TestParseDirect(t *testing.T) {
	yaml := `phase_completed: "Phase 1"
status: CONTINUE
files_changed:
  - main.go
summary: "Direct parse"`

	got, err := ParseDirect(yaml)
	if err != nil {
		t.Fatalf("ParseDirect() error = %v", err)
	}

	if got.PhaseCompleted != "Phase 1" {
		t.Errorf("PhaseCompleted = %q, want %q", got.PhaseCompleted, "Phase 1")
	}
	if got.Status != protocol.StatusContinue {
		t.Errorf("Status = %q, want %q", got.Status, protocol.StatusContinue)
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		name   string
		status *ParsedStatus
		want   bool
	}{
		{
			name: "valid CONTINUE status",
			status: &ParsedStatus{
				Status:  protocol.StatusContinue,
				Summary: "Did work",
			},
			want: true,
		},
		{
			name: "valid DONE status",
			status: &ParsedStatus{
				Status:  protocol.StatusDone,
				Summary: "Finished",
			},
			want: true,
		},
		{
			name: "valid BLOCKED status with error",
			status: &ParsedStatus{
				Status:  protocol.StatusBlocked,
				Summary: "Attempted",
				Error:   "Blocked reason",
			},
			want: true,
		},
		{
			name: "invalid - empty status",
			status: &ParsedStatus{
				Summary: "Did work",
			},
			want: false,
		},
		{
			name: "invalid - unknown status",
			status: &ParsedStatus{
				Status:  "UNKNOWN",
				Summary: "Did work",
			},
			want: false,
		},
		{
			name:   "nil status",
			status: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	if protocol.StatusContinue.String() != "CONTINUE" {
		t.Errorf("protocol.StatusContinue.String() = %q, want CONTINUE", protocol.StatusContinue.String())
	}
	if protocol.StatusDone.String() != "DONE" {
		t.Errorf("protocol.StatusDone.String() = %q, want DONE", protocol.StatusDone.String())
	}
	if protocol.StatusBlocked.String() != "BLOCKED" {
		t.Errorf("protocol.StatusBlocked.String() = %q, want BLOCKED", protocol.StatusBlocked.String())
	}
}

func TestParseQuestionPayload(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantQuestion string
		wantOptions  []string
		wantContext  string
		wantErr      string
	}{
		{
			name: "valid question with options",
			output: "Some output\n" +
				protocol.SignalQuestion + "\n" +
				`{"question": "Which auth method?", "options": ["JWT", "OAuth2", "API keys"]}` + "\n" +
				protocol.SignalEnd + "\n" +
				"more output",
			wantQuestion: "Which auth method?",
			wantOptions:  []string{"JWT", "OAuth2", "API keys"},
		},
		{
			name: "question with context",
			output: protocol.SignalQuestion + "\n" +
				`{"question": "Which database?", "options": ["PostgreSQL", "MySQL"], "context": "I see you have existing MySQL config"}` + "\n" +
				protocol.SignalEnd,
			wantQuestion: "Which database?",
			wantOptions:  []string{"PostgreSQL", "MySQL"},
			wantContext:  "I see you have existing MySQL config",
		},
		{
			name:    "no question signal",
			output:  "Some output without any signals",
			wantErr: "no question signal found",
		},
		{
			name: "missing END marker",
			output: protocol.SignalQuestion + "\n" +
				`{"question": "Test?", "options": ["A", "B"]}`,
			wantErr: "missing END marker",
		},
		{
			name: "invalid JSON",
			output: protocol.SignalQuestion + "\n" +
				"{invalid json}\n" +
				protocol.SignalEnd,
			wantErr: "invalid JSON",
		},
		{
			name: "missing question field",
			output: protocol.SignalQuestion + "\n" +
				`{"options": ["A", "B"]}` + "\n" +
				protocol.SignalEnd,
			wantErr: "missing question field",
		},
		{
			name: "empty options",
			output: protocol.SignalQuestion + "\n" +
				`{"question": "Test?", "options": []}` + "\n" +
				protocol.SignalEnd,
			wantErr: "missing or empty options field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseQuestionPayload(tt.output)

			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("ParseQuestionPayload() expected error containing %q", tt.wantErr)
					return
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Errorf("ParseQuestionPayload() error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseQuestionPayload() unexpected error: %v", err)
				return
			}

			if got.Question != tt.wantQuestion {
				t.Errorf("Question = %q, want %q", got.Question, tt.wantQuestion)
			}

			if len(got.Options) != len(tt.wantOptions) {
				t.Errorf("Options length = %d, want %d", len(got.Options), len(tt.wantOptions))
			} else {
				for i, opt := range got.Options {
					if opt != tt.wantOptions[i] {
						t.Errorf("Options[%d] = %q, want %q", i, opt, tt.wantOptions[i])
					}
				}
			}

			if got.Context != tt.wantContext {
				t.Errorf("Context = %q, want %q", got.Context, tt.wantContext)
			}
		})
	}
}

func TestParsePlanContent(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantContent string
		wantErr     string
	}{
		{
			name: "valid plan content",
			output: "Claude analysis...\n\n" +
				protocol.SignalPlanReady + "\n" +
				"# Plan: Add Authentication\n\n## Tasks\n- [ ] Task 1: Set up auth middleware\n- [ ] Task 2: Add login endpoint\n" +
				protocol.SignalEnd,
			wantContent: `# Plan: Add Authentication

## Tasks
- [ ] Task 1: Set up auth middleware
- [ ] Task 2: Add login endpoint`,
		},
		{
			name:    "no plan ready signal",
			output:  "Some output without plan signal",
			wantErr: "no plan ready signal found",
		},
		{
			name: "missing END marker",
			output: protocol.SignalPlanReady + "\n" +
				"# Plan content",
			wantErr: "missing END marker",
		},
		{
			name: "empty plan content",
			output: protocol.SignalPlanReady + "\n" +
				protocol.SignalEnd,
			wantErr: "empty plan content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlanContent(tt.output)

			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("ParsePlanContent() expected error containing %q", tt.wantErr)
					return
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Errorf("ParsePlanContent() error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("ParsePlanContent() unexpected error: %v", err)
				return
			}

			if got != tt.wantContent {
				t.Errorf("Content = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestHasQuestionSignal(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "has signal",
			output: "Some text " + protocol.SignalQuestion + " more text",
			want:   true,
		},
		{
			name:   "no signal",
			output: "Some text without signal",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasQuestionSignal(tt.output); got != tt.want {
				t.Errorf("HasQuestionSignal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasPlanReadySignal(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "has signal",
			output: "Some text " + protocol.SignalPlanReady + " more text",
			want:   true,
		},
		{
			name:   "no signal",
			output: "Some text without signal",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasPlanReadySignal(tt.output); got != tt.want {
				t.Errorf("HasPlanReadySignal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
