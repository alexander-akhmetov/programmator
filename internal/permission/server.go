package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/alexander-akhmetov/programmator/internal/debug"
)

type Decision string

const (
	DecisionAllow        Decision = "allow"
	DecisionAllowOnce    Decision = "allow_once" // Allow without saving
	DecisionDeny         Decision = "deny"
	DecisionAllowProject Decision = "allow_project"
	DecisionAllowGlobal  Decision = "allow_global"
)

type Request struct {
	SessionID   string         `json:"session_id"`
	ToolName    string         `json:"tool_name"`
	ToolInput   map[string]any `json:"tool_input"`
	ToolUseID   string         `json:"tool_use_id"`
	Description string         `json:"description,omitempty"`
}

type Response struct {
	Decision Decision `json:"decision"`
	Pattern  string   `json:"pattern,omitempty"` // Custom pattern override
}

type HandlerResponse struct {
	Decision Decision
	Pattern  string // If set, use this pattern instead of auto-generated one
}

type RequestHandler func(req *Request) HandlerResponse

type Server struct {
	socketPath      string
	listener        net.Listener
	handler         RequestHandler
	settings        *Settings
	preAllowedMatch []string // patterns that are pre-allowed

	mu       sync.Mutex
	closed   bool
	sessions map[string]map[string]bool // session -> pattern -> allowed
}

func NewServer(projectDir string, handler RequestHandler) (*Server, error) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("programmator-%d.sock", os.Getpid()))

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on socket: %w", err)
	}

	return &Server{
		socketPath: socketPath,
		listener:   listener,
		handler:    handler,
		settings:   NewSettings(projectDir),
		sessions:   make(map[string]map[string]bool),
	}, nil
}

func (s *Server) SocketPath() string {
	return s.socketPath
}

func (s *Server) SetPreAllowed(patterns []string) {
	s.preAllowedMatch = patterns
}

func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		s.Close()
	}()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	if s.listener != nil {
		s.listener.Close()
	}
	os.Remove(s.socketPath)
	return nil
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	debug.Logf("server: new connection")

	var req Request
	decoder := json.NewDecoder(conn)
	if err := decoder.Decode(&req); err != nil {
		debug.Logf("server: failed to decode request: %v", err)
		s.sendResponse(conn, Response{Decision: DecisionDeny})
		return
	}

	debug.Logf("server: received request for tool=%s", req.ToolName)
	decision := s.processRequest(&req)
	debug.Logf("server: decision=%s for tool=%s", decision, req.ToolName)

	s.sendResponse(conn, Response{Decision: decision})
	debug.Logf("server: response sent")
}

func (s *Server) processRequest(req *Request) Decision {
	toolInput := formatToolInput(req.ToolName, req.ToolInput)
	pattern := FormatPattern(req.ToolName, toolInput)

	debug.Logf("server: processing tool=%s pattern=%s", req.ToolName, pattern)

	if s.isSessionAllowed(req.SessionID, pattern) {
		debug.Logf("server: allowed by session cache")
		return DecisionAllow
	}

	if s.settings.IsAllowed(req.ToolName, toolInput) {
		debug.Logf("server: allowed by settings")
		return DecisionAllow
	}

	if s.isPreAllowed(pattern) {
		debug.Logf("server: allowed by pre-allowed patterns")
		return DecisionAllow
	}

	if s.handler == nil {
		debug.Logf("server: no handler, denying")
		return DecisionDeny
	}

	req.Description = toolInput

	debug.Logf("server: calling handler (will block until user responds)...")
	resp := s.handler(req)
	debug.Logf("server: handler returned decision=%s", resp.Decision)

	// Use custom pattern if provided, otherwise use auto-generated
	savePattern := pattern
	if resp.Pattern != "" {
		savePattern = resp.Pattern
	}

	switch resp.Decision {
	case DecisionAllowOnce:
		// Allow without saving - just return allow
		return DecisionAllow
	case DecisionAllow:
		s.addSessionPermission(req.SessionID, savePattern)
	case DecisionAllowProject:
		if err := s.settings.AddPatternToFile(s.settings.projectPath, savePattern); err == nil {
			return DecisionAllow
		}
	case DecisionAllowGlobal:
		if err := s.settings.AddPatternToFile(s.settings.globalPath, savePattern); err == nil {
			return DecisionAllow
		}
	case DecisionDeny:
		// No action needed for deny
	}

	return resp.Decision
}

func (s *Server) isSessionAllowed(sessionID, target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	perms, ok := s.sessions[sessionID]
	if !ok {
		return false
	}

	// Check if any stored pattern matches the target
	for pattern := range perms {
		if MatchPattern(pattern, target) {
			return true
		}
	}
	return false
}

func (s *Server) addSessionPermission(sessionID, pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessions[sessionID] == nil {
		s.sessions[sessionID] = make(map[string]bool)
	}
	s.sessions[sessionID][pattern] = true
}

func (s *Server) isPreAllowed(pattern string) bool {
	for _, allowed := range s.preAllowedMatch {
		if MatchPattern(allowed, pattern) {
			return true
		}
	}
	return false
}

func (s *Server) sendResponse(conn net.Conn, resp Response) {
	encoder := json.NewEncoder(conn)
	_ = encoder.Encode(resp)
}

func formatToolInput(toolName string, input map[string]any) string {
	switch toolName {
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			return cmd
		}
	case "Read", "Write", "Edit":
		if path, ok := input["file_path"].(string); ok {
			return path
		}
	case "WebFetch":
		if url, ok := input["url"].(string); ok {
			return url
		}
	case "Glob":
		if pattern, ok := input["pattern"].(string); ok {
			return pattern
		}
	case "Grep":
		if pattern, ok := input["pattern"].(string); ok {
			return pattern
		}
	}

	if len(input) == 0 {
		return ""
	}

	data, _ := json.Marshal(input)
	return string(data)
}
