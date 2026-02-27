package safety

import (
	"testing"
)

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
		{ExitReasonReviewFailed, "review_failed"},
		{ExitReasonMaxReviewRetries, "max_review_retries"},
	}

	for _, tt := range tests {
		if string(tt.reason) != tt.want {
			t.Errorf("ExitReason %v = %q, want %q", tt.reason, string(tt.reason), tt.want)
		}
	}
}

func TestSetCurrentIterTokens(t *testing.T) {
	t.Run("first call initializes", func(t *testing.T) {
		state := NewState()
		state.SetCurrentIterTokens(100, 50)

		if state.CurrentIterTokens == nil {
			t.Fatal("CurrentIterTokens should not be nil")
		}
		if state.CurrentIterTokens.InputTokens != 100 {
			t.Errorf("InputTokens = %d, want 100", state.CurrentIterTokens.InputTokens)
		}
		if state.CurrentIterTokens.OutputTokens != 50 {
			t.Errorf("OutputTokens = %d, want 50", state.CurrentIterTokens.OutputTokens)
		}
	})

	t.Run("input replaces, output accumulates", func(t *testing.T) {
		state := NewState()
		state.SetCurrentIterTokens(100, 50)
		state.SetCurrentIterTokens(200, 30)

		if state.CurrentIterTokens.InputTokens != 200 {
			t.Errorf("InputTokens = %d, want 200 (replaced)", state.CurrentIterTokens.InputTokens)
		}
		if state.CurrentIterTokens.OutputTokens != 80 {
			t.Errorf("OutputTokens = %d, want 80 (accumulated 50+30)", state.CurrentIterTokens.OutputTokens)
		}
	})
}

func TestFinalizeIterTokens(t *testing.T) {
	t.Run("aggregates by model and clears current", func(t *testing.T) {
		state := NewState()
		state.FinalizeIterTokens("claude-3", 100, 50)

		if state.CurrentIterTokens != nil {
			t.Error("CurrentIterTokens should be nil after finalize")
		}
		mt := state.TokensByModel["claude-3"]
		if mt == nil {
			t.Fatal("expected model entry")
		}
		if mt.InputTokens != 100 || mt.OutputTokens != 50 {
			t.Errorf("tokens = (%d, %d), want (100, 50)", mt.InputTokens, mt.OutputTokens)
		}
	})

	t.Run("accumulates across calls for same model", func(t *testing.T) {
		state := NewState()
		state.FinalizeIterTokens("claude-3", 100, 50)
		state.FinalizeIterTokens("claude-3", 200, 75)

		mt := state.TokensByModel["claude-3"]
		if mt.InputTokens != 300 || mt.OutputTokens != 125 {
			t.Errorf("tokens = (%d, %d), want (300, 125)", mt.InputTokens, mt.OutputTokens)
		}
	})

	t.Run("handles empty model with fallback", func(t *testing.T) {
		state := NewState()
		// No model set and empty string passed — should return early
		state.FinalizeIterTokens("", 100, 50)
		if len(state.TokensByModel) != 0 {
			t.Error("expected no model entries when model is empty")
		}
	})

	t.Run("uses previously set model when empty string passed", func(t *testing.T) {
		state := NewState()
		state.FinalizeIterTokens("claude-3", 100, 50)
		state.FinalizeIterTokens("", 200, 75) // should use "claude-3"

		mt := state.TokensByModel["claude-3"]
		if mt.InputTokens != 300 || mt.OutputTokens != 125 {
			t.Errorf("tokens = (%d, %d), want (300, 125)", mt.InputTokens, mt.OutputTokens)
		}
	})
}

func TestTotalTokens(t *testing.T) {
	t.Run("sums across models", func(t *testing.T) {
		state := NewState()
		state.FinalizeIterTokens("model-a", 100, 50)
		state.FinalizeIterTokens("model-b", 200, 75)

		in, out := state.TotalTokens()
		if in != 300 {
			t.Errorf("total input = %d, want 300", in)
		}
		if out != 125 {
			t.Errorf("total output = %d, want 125", out)
		}
	})

	t.Run("includes current iter tokens", func(t *testing.T) {
		state := NewState()
		state.FinalizeIterTokens("model-a", 100, 50)
		state.SetCurrentIterTokens(80, 20)

		in, out := state.TotalTokens()
		if in != 180 {
			t.Errorf("total input = %d, want 180", in)
		}
		if out != 70 {
			t.Errorf("total output = %d, want 70", out)
		}
	})

	t.Run("zero when empty", func(t *testing.T) {
		state := NewState()
		in, out := state.TotalTokens()
		if in != 0 || out != 0 {
			t.Errorf("total = (%d, %d), want (0, 0)", in, out)
		}
	})
}

func TestReviewPhase(t *testing.T) {
	t.Run("RecordReviewIteration increments", func(t *testing.T) {
		state := NewState()
		state.RecordReviewIteration()
		state.RecordReviewIteration()
		if state.ReviewIterations != 2 {
			t.Errorf("ReviewIterations = %d, want 2", state.ReviewIterations)
		}
	})

	t.Run("EnterReviewPhase sets flag", func(t *testing.T) {
		state := NewState()
		if state.InReviewPhase {
			t.Error("InReviewPhase should be false initially")
		}
		state.EnterReviewPhase()
		if !state.InReviewPhase {
			t.Error("InReviewPhase should be true after EnterReviewPhase")
		}
	})

	t.Run("ExitReviewPhase clears flag", func(t *testing.T) {
		state := NewState()
		state.EnterReviewPhase()
		state.ExitReviewPhase()
		if state.InReviewPhase {
			t.Error("InReviewPhase should be false after ExitReviewPhase")
		}
	})
}

func TestCheck_MaxReviewRetries(t *testing.T) {
	cfg := Config{
		MaxIterations:       50,
		StagnationLimit:     3,
		MaxReviewIterations: 3,
	}

	t.Run("triggers when in review phase at limit", func(t *testing.T) {
		state := NewState()
		state.Iteration = 1
		state.InReviewPhase = true
		state.ReviewIterations = 3

		result := Check(cfg, state)
		if !result.ShouldExit {
			t.Error("ShouldExit = false, want true")
		}
		if result.Reason != ExitReasonMaxReviewRetries {
			t.Errorf("Reason = %v, want %v", result.Reason, ExitReasonMaxReviewRetries)
		}
	})

	t.Run("does not trigger when not in review phase", func(t *testing.T) {
		state := NewState()
		state.Iteration = 1
		state.InReviewPhase = false
		state.ReviewIterations = 5

		result := Check(cfg, state)
		if result.ShouldExit {
			t.Error("ShouldExit = true, want false (not in review phase)")
		}
	})

	t.Run("does not trigger when below limit", func(t *testing.T) {
		state := NewState()
		state.Iteration = 1
		state.InReviewPhase = true
		state.ReviewIterations = 2

		result := Check(cfg, state)
		if result.ShouldExit {
			t.Error("ShouldExit = true, want false (below limit)")
		}
	})
}
