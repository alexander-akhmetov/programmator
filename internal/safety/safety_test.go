package safety

import (
	"os"
	"testing"
)

func TestConfigFromEnv_Defaults(t *testing.T) {
	os.Clearenv()
	cfg := ConfigFromEnv()

	if cfg.MaxIterations != DefaultMaxIterations {
		t.Errorf("MaxIterations = %d, want %d", cfg.MaxIterations, DefaultMaxIterations)
	}
	if cfg.StagnationLimit != DefaultStagnationLimit {
		t.Errorf("StagnationLimit = %d, want %d", cfg.StagnationLimit, DefaultStagnationLimit)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %d, want %d", cfg.Timeout, DefaultTimeout)
	}
	if cfg.ClaudeFlags != "" {
		t.Errorf("ClaudeFlags = %q, want %q", cfg.ClaudeFlags, "")
	}
	if cfg.ClaudeConfigDir != "" {
		t.Errorf("ClaudeConfigDir = %q, want %q", cfg.ClaudeConfigDir, "")
	}
}

func TestConfigFromEnv_CustomValues(t *testing.T) {
	os.Setenv("PROGRAMMATOR_MAX_ITERATIONS", "100")
	os.Setenv("PROGRAMMATOR_STAGNATION_LIMIT", "5")
	os.Setenv("PROGRAMMATOR_TIMEOUT", "1800")
	os.Setenv("PROGRAMMATOR_CLAUDE_FLAGS", "--verbose")
	os.Setenv("CLAUDE_CONFIG_DIR", "/home/user/.claude-personal")
	defer os.Clearenv()

	cfg := ConfigFromEnv()

	if cfg.MaxIterations != 100 {
		t.Errorf("MaxIterations = %d, want %d", cfg.MaxIterations, 100)
	}
	if cfg.StagnationLimit != 5 {
		t.Errorf("StagnationLimit = %d, want %d", cfg.StagnationLimit, 5)
	}
	if cfg.Timeout != 1800 {
		t.Errorf("Timeout = %d, want %d", cfg.Timeout, 1800)
	}
	if cfg.ClaudeFlags != "--verbose" {
		t.Errorf("ClaudeFlags = %q, want %q", cfg.ClaudeFlags, "--verbose")
	}
	if cfg.ClaudeConfigDir != "/home/user/.claude-personal" {
		t.Errorf("ClaudeConfigDir = %q, want %q", cfg.ClaudeConfigDir, "/home/user/.claude-personal")
	}
}

func TestNewState(t *testing.T) {
	state := NewState()

	if state.Iteration != 0 {
		t.Errorf("Iteration = %d, want 0", state.Iteration)
	}
	if state.ConsecutiveNoChanges != 0 {
		t.Errorf("ConsecutiveNoChanges = %d, want 0", state.ConsecutiveNoChanges)
	}
	if state.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", state.ConsecutiveErrors)
	}
	if state.FilesChangedHistory == nil {
		t.Error("FilesChangedHistory should not be nil")
	}
	if state.TotalFilesChanged == nil {
		t.Error("TotalFilesChanged should not be nil")
	}
}

func TestState_RecordIteration_WithFiles(t *testing.T) {
	state := NewState()
	state.RecordIteration([]string{"file1.go", "file2.go"}, "")

	// Iteration is managed by the loop, not RecordIteration
	if state.ConsecutiveNoChanges != 0 {
		t.Errorf("ConsecutiveNoChanges = %d, want 0", state.ConsecutiveNoChanges)
	}
	if len(state.FilesChangedHistory) != 1 {
		t.Errorf("FilesChangedHistory len = %d, want 1", len(state.FilesChangedHistory))
	}
	if len(state.TotalFilesChanged) != 2 {
		t.Errorf("TotalFilesChanged len = %d, want 2", len(state.TotalFilesChanged))
	}
}

func TestState_RecordIteration_NoFiles(t *testing.T) {
	state := NewState()
	state.RecordIteration([]string{}, "")
	state.RecordIteration([]string{}, "")

	if state.ConsecutiveNoChanges != 2 {
		t.Errorf("ConsecutiveNoChanges = %d, want 2", state.ConsecutiveNoChanges)
	}
}

func TestState_RecordIteration_ResetStagnation(t *testing.T) {
	state := NewState()
	state.RecordIteration([]string{}, "")
	state.RecordIteration([]string{}, "")
	state.RecordIteration([]string{"file.go"}, "")

	if state.ConsecutiveNoChanges != 0 {
		t.Errorf("ConsecutiveNoChanges = %d, want 0 after file change", state.ConsecutiveNoChanges)
	}
}

func TestState_RecordIteration_ConsecutiveErrors(t *testing.T) {
	state := NewState()

	state.RecordIteration([]string{}, "error A")
	if state.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", state.ConsecutiveErrors)
	}

	state.RecordIteration([]string{}, "error A")
	if state.ConsecutiveErrors != 2 {
		t.Errorf("ConsecutiveErrors = %d, want 2", state.ConsecutiveErrors)
	}

	state.RecordIteration([]string{}, "error B")
	if state.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1 (different error)", state.ConsecutiveErrors)
	}

	state.RecordIteration([]string{}, "")
	if state.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0 (no error)", state.ConsecutiveErrors)
	}
}

func TestCheck_MaxIterations(t *testing.T) {
	cfg := Config{MaxIterations: 5, StagnationLimit: 3}
	state := NewState()
	state.Iteration = 6 // Must be > MaxIterations to trigger exit

	result := Check(cfg, state)

	if !result.ShouldExit {
		t.Error("ShouldExit = false, want true")
	}
	if result.Reason != ExitReasonMaxIterations {
		t.Errorf("Reason = %v, want %v", result.Reason, ExitReasonMaxIterations)
	}
}

func TestCheck_Stagnation(t *testing.T) {
	cfg := Config{MaxIterations: 50, StagnationLimit: 3}
	state := NewState()
	state.Iteration = 3
	state.ConsecutiveNoChanges = 3

	result := Check(cfg, state)

	if !result.ShouldExit {
		t.Error("ShouldExit = false, want true")
	}
	if result.Reason != ExitReasonStagnation {
		t.Errorf("Reason = %v, want %v", result.Reason, ExitReasonStagnation)
	}
}

func TestCheck_ConsecutiveErrors(t *testing.T) {
	cfg := Config{MaxIterations: 50, StagnationLimit: 3}
	state := NewState()
	state.Iteration = 3
	state.ConsecutiveErrors = 3

	result := Check(cfg, state)

	if !result.ShouldExit {
		t.Error("ShouldExit = false, want true")
	}
	if result.Reason != ExitReasonBlocked {
		t.Errorf("Reason = %v, want %v", result.Reason, ExitReasonBlocked)
	}
}

func TestCheck_Continue(t *testing.T) {
	cfg := Config{MaxIterations: 50, StagnationLimit: 3}
	state := NewState()
	state.Iteration = 1
	state.ConsecutiveNoChanges = 1
	state.ConsecutiveErrors = 1

	result := Check(cfg, state)

	if result.ShouldExit {
		t.Error("ShouldExit = true, want false")
	}
}

func TestExitReasonValues(t *testing.T) {
	tests := []struct {
		reason ExitReason
		want   string
	}{
		{ExitReasonComplete, "complete"},
		{ExitReasonMaxIterations, "max_iterations"},
		{ExitReasonStagnation, "stagnation"},
		{ExitReasonBlocked, "blocked"},
		{ExitReasonError, "error"},
		{ExitReasonUserInterrupt, "user_interrupt"},
	}

	for _, tt := range tests {
		if string(tt.reason) != tt.want {
			t.Errorf("ExitReason %v = %q, want %q", tt.reason, string(tt.reason), tt.want)
		}
	}
}
