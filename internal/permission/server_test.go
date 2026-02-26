package permission

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForSocket polls the socket until it's ready for connections.
// Uses a 5-second timeout which is sufficient for test server startup.
func waitForSocket(t *testing.T, socketPath string) {
	t.Helper()
	const timeout = 5 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s did not become ready within %v", socketPath, timeout)
}

func TestServerCreation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	server, err := NewServer(tmpDir, nil)
	require.NoError(t, err)
	defer server.Close()

	assert.NotEmpty(t, server.SocketPath())
	assert.FileExists(t, server.SocketPath())
}

func TestServerAllowFromSettings(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	settingsDir := filepath.Join(tmpDir, ".claude")
	require.NoError(t, os.MkdirAll(settingsDir, 0755))

	settings := claudeSettings{
		Permissions: &permissionsBlock{
			Allow: []string{"Bash(git:*)"},
		},
	}
	data, _ := json.Marshal(settings)
	require.NoError(t, os.WriteFile(filepath.Join(settingsDir, "settings.local.json"), data, 0644))

	handlerCalled := false
	server, err := NewServer(tmpDir, func(_ *Request) HandlerResponse {
		handlerCalled = true
		return HandlerResponse{Decision: DecisionDeny}
	})
	require.NoError(t, err)
	defer server.Close()

	ctx := t.Context()

	go func() { _ = server.Serve(ctx) }()
	waitForSocket(t, server.SocketPath())

	conn, err := net.Dial("unix", server.SocketPath())
	require.NoError(t, err)
	defer conn.Close()

	req := Request{
		SessionID: "test-session",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "git status"},
	}
	encoder := json.NewEncoder(conn)
	require.NoError(t, encoder.Encode(req))

	var resp Response
	decoder := json.NewDecoder(conn)
	require.NoError(t, decoder.Decode(&resp))

	assert.Equal(t, DecisionAllow, resp.Decision)
	assert.False(t, handlerCalled, "handler should not be called for pre-allowed tools")
}

func TestServerDenyWhenNotAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	handlerCalled := false
	server, err := NewServer(tmpDir, func(_ *Request) HandlerResponse {
		handlerCalled = true
		return HandlerResponse{Decision: DecisionDeny}
	})
	require.NoError(t, err)
	defer server.Close()

	ctx := t.Context()

	go func() { _ = server.Serve(ctx) }()
	waitForSocket(t, server.SocketPath())

	conn, err := net.Dial("unix", server.SocketPath())
	require.NoError(t, err)
	defer conn.Close()

	req := Request{
		SessionID: "test-session",
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "rm -rf /"},
	}
	encoder := json.NewEncoder(conn)
	require.NoError(t, encoder.Encode(req))

	var resp Response
	decoder := json.NewDecoder(conn)
	require.NoError(t, decoder.Decode(&resp))

	assert.Equal(t, DecisionDeny, resp.Decision)
	assert.True(t, handlerCalled, "handler should be called")
}

func TestServerSessionPermission(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	callCount := 0
	server, err := NewServer(tmpDir, func(_ *Request) HandlerResponse {
		callCount++
		return HandlerResponse{Decision: DecisionAllow}
	})
	require.NoError(t, err)
	defer server.Close()

	// Override global settings to ensure test isolation
	server.settings.globalPath = filepath.Join(tmpDir, "nonexistent-global-settings.json")

	ctx := t.Context()

	go func() { _ = server.Serve(ctx) }()
	waitForSocket(t, server.SocketPath())

	makeRequest := func() Response {
		conn, err := net.Dial("unix", server.SocketPath())
		require.NoError(t, err)
		defer conn.Close()

		req := Request{
			SessionID: "test-session",
			ToolName:  "Bash",
			ToolInput: map[string]any{"command": "test-unique-cmd"},
		}
		encoder := json.NewEncoder(conn)
		require.NoError(t, encoder.Encode(req))

		var resp Response
		decoder := json.NewDecoder(conn)
		require.NoError(t, decoder.Decode(&resp))
		return resp
	}

	resp1 := makeRequest()
	assert.Equal(t, DecisionAllow, resp1.Decision)
	assert.Equal(t, 1, callCount, "handler called first time")

	resp2 := makeRequest()
	assert.Equal(t, DecisionAllow, resp2.Decision)
	assert.Equal(t, 1, callCount, "handler not called second time - session cached")
}

func TestServerSessionPermissionWildcard(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	callCount := 0
	server, err := NewServer(tmpDir, func(_ *Request) HandlerResponse {
		callCount++
		// Return a wildcard pattern (like "Commands starting with 'yarn'")
		return HandlerResponse{Decision: DecisionAllow, Pattern: "Bash(yarn:*)"}
	})
	require.NoError(t, err)
	defer server.Close()

	ctx := t.Context()

	go func() { _ = server.Serve(ctx) }()
	waitForSocket(t, server.SocketPath())

	makeRequest := func(cmd string) Response {
		conn, err := net.Dial("unix", server.SocketPath())
		require.NoError(t, err)
		defer conn.Close()

		req := Request{
			SessionID: "test-session",
			ToolName:  "Bash",
			ToolInput: map[string]any{"command": cmd},
		}
		encoder := json.NewEncoder(conn)
		require.NoError(t, encoder.Encode(req))

		var resp Response
		decoder := json.NewDecoder(conn)
		require.NoError(t, decoder.Decode(&resp))
		return resp
	}

	// First yarn command - should call handler
	resp1 := makeRequest("yarn install")
	assert.Equal(t, DecisionAllow, resp1.Decision)
	assert.Equal(t, 1, callCount, "handler called first time")

	// Second yarn command (different) - should be cached via wildcard
	resp2 := makeRequest("yarn test")
	assert.Equal(t, DecisionAllow, resp2.Decision)
	assert.Equal(t, 1, callCount, "handler not called - wildcard pattern matched")

	// Third yarn command (different) - should still be cached
	resp3 := makeRequest("yarn build --production")
	assert.Equal(t, DecisionAllow, resp3.Decision)
	assert.Equal(t, 1, callCount, "handler not called - wildcard pattern matched")

	// Non-yarn command - should call handler
	resp4 := makeRequest("npm install")
	assert.Equal(t, DecisionAllow, resp4.Decision)
	assert.Equal(t, 2, callCount, "handler called for different command")
}

func TestServerPreAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	handlerCalled := false
	server, err := NewServer(tmpDir, func(_ *Request) HandlerResponse {
		handlerCalled = true
		return HandlerResponse{Decision: DecisionDeny}
	})
	require.NoError(t, err)
	defer server.Close()

	server.SetPreAllowed([]string{"Bash(git:*)", "Read"})

	ctx := t.Context()

	go func() { _ = server.Serve(ctx) }()
	waitForSocket(t, server.SocketPath())

	makeRequest := func(toolName string, input map[string]any) Response {
		conn, err := net.Dial("unix", server.SocketPath())
		require.NoError(t, err)
		defer conn.Close()

		req := Request{
			SessionID: "test-session",
			ToolName:  toolName,
			ToolInput: input,
		}
		encoder := json.NewEncoder(conn)
		require.NoError(t, encoder.Encode(req))

		var resp Response
		decoder := json.NewDecoder(conn)
		require.NoError(t, decoder.Decode(&resp))
		return resp
	}

	resp1 := makeRequest("Bash", map[string]any{"command": "git status"})
	assert.Equal(t, DecisionAllow, resp1.Decision, "git command should be pre-allowed")

	resp2 := makeRequest("Read", map[string]any{"file_path": "/any/file"})
	assert.Equal(t, DecisionAllow, resp2.Decision, "Read should be pre-allowed")

	resp3 := makeRequest("Write", map[string]any{"file_path": "/any/file"})
	assert.Equal(t, DecisionDeny, resp3.Decision, "Write should not be pre-allowed")

	assert.True(t, handlerCalled, "handler should be called for non-pre-allowed tools")
}

func TestFormatToolInput(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		input    map[string]any
		expected string
	}{
		{
			name:     "bash command",
			toolName: "Bash",
			input:    map[string]any{"command": "git status"},
			expected: "git status",
		},
		{
			name:     "read file path",
			toolName: "Read",
			input:    map[string]any{"file_path": "/path/to/file"},
			expected: "/path/to/file",
		},
		{
			name:     "write file path",
			toolName: "Write",
			input:    map[string]any{"file_path": "/path/to/file", "content": "data"},
			expected: "/path/to/file",
		},
		{
			name:     "web fetch url",
			toolName: "WebFetch",
			input:    map[string]any{"url": "https://example.com"},
			expected: "https://example.com",
		},
		{
			name:     "glob pattern",
			toolName: "Glob",
			input:    map[string]any{"pattern": "**/*.go"},
			expected: "**/*.go",
		},
		{
			name:     "grep pattern",
			toolName: "Grep",
			input:    map[string]any{"pattern": "TODO"},
			expected: "TODO",
		},
		{
			name:     "empty input",
			toolName: "Unknown",
			input:    map[string]any{},
			expected: "",
		},
		{
			name:     "unknown tool fallback to json",
			toolName: "Custom",
			input:    map[string]any{"foo": "bar"},
			expected: `{"foo":"bar"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatToolInput(tt.toolName, tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
