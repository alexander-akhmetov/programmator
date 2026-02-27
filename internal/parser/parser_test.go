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

func TestParsePhaseCompletedIndex(t *testing.T) {
	idx3 := 3
	idx1 := 1

	tests := []struct {
		name      string
		output    string
		wantIndex *int
		wantPhase string
	}{
		{
			name: "index present",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  phase_completed_index: 3
  status: CONTINUE
  files_changed: []
  summary: "Done"
`,
			wantIndex: &idx3,
			wantPhase: "Phase 1",
		},
		{
			name: "index absent",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: CONTINUE
  files_changed: []
  summary: "Done"
`,
			wantIndex: nil,
			wantPhase: "Phase 1",
		},
		{
			name: "index only, no phase name",
			output: `PROGRAMMATOR_STATUS:
  phase_completed: null
  phase_completed_index: 1
  status: CONTINUE
  files_changed: []
  summary: "Done"
`,
			wantIndex: &idx1,
			wantPhase: "",
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
			if got.PhaseCompleted != tt.wantPhase {
				t.Errorf("PhaseCompleted = %q, want %q", got.PhaseCompleted, tt.wantPhase)
			}
			if tt.wantIndex == nil {
				if got.PhaseCompletedIndex != nil {
					t.Errorf("PhaseCompletedIndex = %d, want nil", *got.PhaseCompletedIndex)
				}
			} else {
				if got.PhaseCompletedIndex == nil {
					t.Errorf("PhaseCompletedIndex = nil, want %d", *tt.wantIndex)
				} else if *got.PhaseCompletedIndex != *tt.wantIndex {
					t.Errorf("PhaseCompletedIndex = %d, want %d", *got.PhaseCompletedIndex, *tt.wantIndex)
				}
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
