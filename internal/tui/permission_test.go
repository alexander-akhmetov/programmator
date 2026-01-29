package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/worksonmyai/programmator/internal/permission"
)

func TestBuildAllowOptions_FileTools(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		description   string
		wantContains  []string
		wantOptionCnt int
		wantDirGlob   string // expected directory pattern suffix
	}{
		{
			name:        "Read file uses glob pattern",
			toolName:    "Read",
			description: "/tmp/project/src/main.go",
			wantContains: []string{
				"This path",
				"Directory",
				"All Read operations",
			},
			wantOptionCnt: 3,
			wantDirGlob:   "Read(/tmp/project/src/**)",
		},
		{
			name:        "Write file uses glob pattern",
			toolName:    "Write",
			description: "/tmp/test.txt",
			wantContains: []string{
				"This path",
				"Directory",
				"All Write operations",
			},
			wantOptionCnt: 3,
			wantDirGlob:   "Write(/tmp/**)",
		},
		{
			name:        "Edit file uses glob pattern",
			toolName:    "Edit",
			description: "/home/user/file.go",
			wantContains: []string{
				"This path",
				"Directory",
				"All Edit operations",
			},
			wantOptionCnt: 3,
			wantDirGlob:   "Edit(/home/user/**)",
		},
		{
			name:        "Glob uses prefix pattern",
			toolName:    "Glob",
			description: "/tmp/project/src/main.go",
			wantContains: []string{
				"This path",
				"Directory",
				"All Glob operations",
			},
			wantOptionCnt: 3,
			wantDirGlob:   "Glob(/tmp/project/src:*)",
		},
		{
			name:        "Grep uses prefix pattern",
			toolName:    "Grep",
			description: "/tmp/project/src/main.go",
			wantContains: []string{
				"This path",
				"Directory",
				"All Grep operations",
			},
			wantOptionCnt: 3,
			wantDirGlob:   "Grep(/tmp/project/src:*)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &permission.Request{
				ToolName:    tt.toolName,
				Description: tt.description,
			}
			respChan := make(chan permission.HandlerResponse, 1)
			dialog := NewPermissionDialog(req, respChan)

			assert.Len(t, dialog.allowOptions, tt.wantOptionCnt)

			labels := make([]string, len(dialog.allowOptions))
			for i, opt := range dialog.allowOptions {
				labels[i] = opt.label
			}
			allLabels := strings.Join(labels, " ")

			for _, want := range tt.wantContains {
				assert.Contains(t, allLabels, want, "labels should contain: %s", want)
			}

			// Verify directory pattern uses correct syntax
			if tt.wantDirGlob != "" {
				found := false
				for _, opt := range dialog.allowOptions {
					if opt.pattern == tt.wantDirGlob {
						found = true
						break
					}
				}
				assert.True(t, found, "should have directory pattern: %s", tt.wantDirGlob)
			}
		})
	}
}

func TestBuildAllowOptions_Bash(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantLabels  []string
		wantPattern string
	}{
		{
			name:        "git command with subcommand",
			description: "git status",
			wantLabels: []string{
				"This exact command",
				"'git status ...' commands",
				"All 'git' commands",
				"All Bash commands",
			},
			wantPattern: "Bash(git status)",
		},
		{
			name:        "npm command with subcommand",
			description: "npm install lodash",
			wantLabels: []string{
				"This exact command",
				"'npm install ...' commands",
				"All 'npm' commands",
				"All Bash commands",
			},
			wantPattern: "Bash(npm install lodash)",
		},
		{
			name:        "command with flag (no subcommand option)",
			description: "ls -la",
			wantLabels: []string{
				"This exact command",
				"All 'ls' commands",
				"All Bash commands",
			},
			wantPattern: "Bash(ls -la)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &permission.Request{
				ToolName:    "Bash",
				Description: tt.description,
			}
			respChan := make(chan permission.HandlerResponse, 1)
			dialog := NewPermissionDialog(req, respChan)

			labels := make([]string, len(dialog.allowOptions))
			for i, opt := range dialog.allowOptions {
				labels[i] = opt.label
			}

			for _, want := range tt.wantLabels {
				assert.Contains(t, labels, want, "should contain label: %s", want)
			}

			assert.Equal(t, tt.wantPattern, dialog.allowOptions[0].pattern)
		})
	}
}

func TestBuildAllowOptions_GenericTool(t *testing.T) {
	req := &permission.Request{
		ToolName:    "CustomTool",
		Description: "some input",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	require.Len(t, dialog.allowOptions, 2)
	assert.Equal(t, "This request", dialog.allowOptions[0].label)
	assert.Equal(t, "All CustomTool operations", dialog.allowOptions[1].label)
}

func TestHandleKey_Navigation(t *testing.T) {
	req := &permission.Request{
		ToolName:    "Read",
		Description: "/path/to/file",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	// Default: cursor=0, allowIdx=0, scope=Once
	assert.Equal(t, 0, dialog.cursor)
	assert.Equal(t, 0, dialog.allowIdx)
	assert.Equal(t, scopeOnce, dialog.scope)

	// Down moves cursor
	closed := dialog.HandleKey("down")
	assert.False(t, closed)
	assert.Equal(t, 1, dialog.cursor)

	closed = dialog.HandleKey("j")
	assert.False(t, closed)
	assert.Equal(t, 2, dialog.cursor)

	// Up moves cursor back
	closed = dialog.HandleKey("up")
	assert.False(t, closed)
	assert.Equal(t, 1, dialog.cursor)

	closed = dialog.HandleKey("k")
	assert.False(t, closed)
	assert.Equal(t, 0, dialog.cursor)

	// Can't go below 0
	closed = dialog.HandleKey("up")
	assert.False(t, closed)
	assert.Equal(t, 0, dialog.cursor, "should not go below 0")

	// Space selects allow option
	dialog.cursor = 1
	dialog.HandleKey(" ")
	assert.Equal(t, 1, dialog.allowIdx)

	// Move to scope section (after allow options)
	numAllowOptions := len(dialog.allowOptions)
	dialog.cursor = numAllowOptions + 1 // Session
	dialog.HandleKey(" ")
	assert.Equal(t, scopeSession, dialog.scope)
}

func TestHandleKey_ScopeToggle(t *testing.T) {
	req := &permission.Request{
		ToolName:    "Read",
		Description: "/path/to/file",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	// Default is scopeOnce
	assert.Equal(t, scopeOnce, dialog.scope)

	numAllowOptions := len(dialog.allowOptions)

	// Move cursor to scope section and select different scopes
	dialog.cursor = numAllowOptions + 1 // Session
	dialog.HandleKey(" ")
	assert.Equal(t, scopeSession, dialog.scope)

	dialog.cursor = numAllowOptions + 2 // Project
	dialog.HandleKey(" ")
	assert.Equal(t, scopeProject, dialog.scope)

	dialog.cursor = numAllowOptions + 3 // Global
	dialog.HandleKey(" ")
	assert.Equal(t, scopeGlobal, dialog.scope)

	dialog.cursor = numAllowOptions + 0 // Once
	dialog.HandleKey(" ")
	assert.Equal(t, scopeOnce, dialog.scope)
}

func TestHandleKey_Respond(t *testing.T) {
	tests := []struct {
		name         string
		scope        scopeType
		wantDecision permission.Decision
	}{
		{
			name:         "once scope",
			scope:        scopeOnce,
			wantDecision: permission.DecisionAllowOnce,
		},
		{
			name:         "session scope",
			scope:        scopeSession,
			wantDecision: permission.DecisionAllow,
		},
		{
			name:         "project scope",
			scope:        scopeProject,
			wantDecision: permission.DecisionAllowProject,
		},
		{
			name:         "global scope",
			scope:        scopeGlobal,
			wantDecision: permission.DecisionAllowGlobal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &permission.Request{
				ToolName:    "Bash",
				Description: "git status",
			}
			respChan := make(chan permission.HandlerResponse, 1)
			dialog := NewPermissionDialog(req, respChan)
			dialog.scope = tt.scope

			closed := dialog.HandleKey("enter")
			assert.True(t, closed)

			resp := <-respChan
			assert.Equal(t, tt.wantDecision, resp.Decision)
			assert.Equal(t, "Bash(git status)", resp.Pattern)
		})
	}
}

func TestHandleKey_Deny(t *testing.T) {
	denyKeys := []string{"d", "n", "escape"}

	for _, key := range denyKeys {
		t.Run(key, func(t *testing.T) {
			req := &permission.Request{
				ToolName:    "Bash",
				Description: "rm -rf /",
			}
			respChan := make(chan permission.HandlerResponse, 1)
			dialog := NewPermissionDialog(req, respChan)

			closed := dialog.HandleKey(key)
			assert.True(t, closed)

			resp := <-respChan
			assert.Equal(t, permission.DecisionDeny, resp.Decision)
		})
	}
}

func TestGetSelectedPattern(t *testing.T) {
	req := &permission.Request{
		ToolName:    "Bash",
		Description: "git status",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	// idx 0: exact command
	assert.Equal(t, "Bash(git status)", dialog.GetSelectedPattern())

	// idx 1: subcommand pattern (git status:*)
	dialog.allowIdx = 1
	assert.Equal(t, "Bash(git status:*)", dialog.GetSelectedPattern())

	// idx 2: base command pattern (git:*)
	dialog.allowIdx = 2
	assert.Equal(t, "Bash(git:*)", dialog.GetSelectedPattern())

	// idx 3: all Bash
	dialog.allowIdx = 3
	assert.Equal(t, "Bash", dialog.GetSelectedPattern())
}

func TestRenderDialog(t *testing.T) {
	req := &permission.Request{
		ToolName:    "Bash",
		Description: "git status",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	output := dialog.renderDialog(80)

	assert.Contains(t, output, "PERMISSION REQUEST")
	assert.Contains(t, output, "Bash")
	assert.Contains(t, output, "git status")
	assert.Contains(t, output, "Session")
	assert.Contains(t, output, "Project")
	assert.Contains(t, output, "Global")
}

func TestView(t *testing.T) {
	req := &permission.Request{
		ToolName:    "Read",
		Description: "/path/to/file.go",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	output := dialog.View(100, 30)

	assert.NotEmpty(t, output)
	assert.Contains(t, output, "PERMISSION REQUEST")
}
