package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexander-akhmetov/programmator/internal/permission"
)

func TestBuildAllowOptions_FileTools(t *testing.T) {
	tests := []struct {
		name          string
		toolName      string
		description   string
		wantContains  []string
		wantOptionCnt int
	}{
		{
			name:        "Read file",
			toolName:    "Read",
			description: "/tmp/project/src/main.go",
			wantContains: []string{
				"This path",
				"Directory",
				"All Read operations",
			},
			wantOptionCnt: 3,
		},
		{
			name:        "Write file",
			toolName:    "Write",
			description: "/tmp/test.txt",
			wantContains: []string{
				"This path",
				"Directory",
				"All Write operations",
			},
			wantOptionCnt: 3,
		},
		{
			name:        "Edit file",
			toolName:    "Edit",
			description: "/home/user/file.go",
			wantContains: []string{
				"This path",
				"Directory",
				"All Edit operations",
			},
			wantOptionCnt: 3,
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
			name:        "git command",
			description: "git status",
			wantLabels: []string{
				"This exact command",
				"Commands starting with 'git'",
				"All Bash commands",
			},
			wantPattern: "Bash(git status)",
		},
		{
			name:        "npm command",
			description: "npm install lodash",
			wantLabels: []string{
				"This exact command",
				"Commands starting with 'npm'",
				"All Bash commands",
			},
			wantPattern: "Bash(npm install lodash)",
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

	assert.Equal(t, 0, dialog.allowIdx)

	closed := dialog.HandleKey("down")
	assert.False(t, closed)
	assert.Equal(t, 1, dialog.allowIdx)

	closed = dialog.HandleKey("j")
	assert.False(t, closed)
	assert.Equal(t, 2, dialog.allowIdx)

	closed = dialog.HandleKey("up")
	assert.False(t, closed)
	assert.Equal(t, 1, dialog.allowIdx)

	closed = dialog.HandleKey("k")
	assert.False(t, closed)
	assert.Equal(t, 0, dialog.allowIdx)

	closed = dialog.HandleKey("up")
	assert.False(t, closed)
	assert.Equal(t, 0, dialog.allowIdx, "should not go below 0")
}

func TestHandleKey_ScopeToggle(t *testing.T) {
	req := &permission.Request{
		ToolName:    "Read",
		Description: "/path/to/file",
	}
	respChan := make(chan permission.HandlerResponse, 1)
	dialog := NewPermissionDialog(req, respChan)

	// Default is now scopeOnce
	assert.Equal(t, scopeOnce, dialog.scope)

	dialog.HandleKey("tab")
	assert.Equal(t, scopeSession, dialog.scope)

	dialog.HandleKey("tab")
	assert.Equal(t, scopeProject, dialog.scope)

	dialog.HandleKey("tab")
	assert.Equal(t, scopeGlobal, dialog.scope)

	dialog.HandleKey("tab")
	assert.Equal(t, scopeOnce, dialog.scope, "should wrap around")

	// Left goes backward: Once -> Global
	dialog.HandleKey("left")
	assert.Equal(t, scopeGlobal, dialog.scope)

	// Right goes forward: Global -> Once
	dialog.HandleKey("right")
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

	assert.Equal(t, "Bash(git status)", dialog.GetSelectedPattern())

	dialog.allowIdx = 1
	assert.Equal(t, "Bash(git:*)", dialog.GetSelectedPattern())

	dialog.allowIdx = 2
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
