// Package safety implements exit condition checks for the main loop.
package safety

import (
	"os"
	"strconv"
	"time"
)

const (
	DefaultMaxIterations       = 50
	DefaultStagnationLimit     = 3
	DefaultTimeout             = 900 // seconds
	DefaultMaxReviewIterations = 3
)

type ExitReason string

const (
	ExitReasonComplete         ExitReason = "complete"
	ExitReasonMaxIterations    ExitReason = "max_iterations"
	ExitReasonStagnation       ExitReason = "stagnation"
	ExitReasonBlocked          ExitReason = "blocked"
	ExitReasonError            ExitReason = "error"
	ExitReasonUserInterrupt    ExitReason = "user_interrupt"
	ExitReasonReviewFailed     ExitReason = "review_failed"
	ExitReasonMaxReviewRetries ExitReason = "max_review_retries"
)

type Config struct {
	MaxIterations       int
	StagnationLimit     int
	Timeout             int
	ClaudeFlags         string
	ClaudeConfigDir     string
	MaxReviewIterations int
}

func ConfigFromEnv() Config {
	cfg := Config{
		MaxIterations:       DefaultMaxIterations,
		StagnationLimit:     DefaultStagnationLimit,
		Timeout:             DefaultTimeout,
		ClaudeFlags:         "",
		MaxReviewIterations: DefaultMaxReviewIterations,
	}

	if v := os.Getenv("PROGRAMMATOR_MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxIterations = n
		}
	}

	if v := os.Getenv("PROGRAMMATOR_STAGNATION_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.StagnationLimit = n
		}
	}

	if v := os.Getenv("PROGRAMMATOR_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Timeout = n
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CLAUDE_FLAGS"); v != "" {
		cfg.ClaudeFlags = v
	}

	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		cfg.ClaudeConfigDir = v
	}

	if v := os.Getenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxReviewIterations = n
		}
	}

	return cfg
}

type ModelTokens struct {
	InputTokens  int
	OutputTokens int
}

type State struct {
	Iteration            int
	ConsecutiveNoChanges int
	LastError            string
	ConsecutiveErrors    int
	FilesChangedHistory  [][]string
	TotalFilesChanged    map[string]struct{}
	StartTime            time.Time
	Model                string
	TokensByModel        map[string]*ModelTokens
	CurrentIterTokens    *ModelTokens // live tokens for current iteration
	ReviewIterations     int          // number of review iterations performed
	InReviewPhase        bool         // whether we're currently in review phase
}

func NewState() *State {
	return &State{
		FilesChangedHistory: make([][]string, 0),
		TotalFilesChanged:   make(map[string]struct{}),
		StartTime:           time.Now(),
		TokensByModel:       make(map[string]*ModelTokens),
	}
}

func (s *State) RecordIteration(filesChanged []string, err string) {
	s.FilesChangedHistory = append(s.FilesChangedHistory, filesChanged)

	if len(filesChanged) > 0 {
		s.ConsecutiveNoChanges = 0
		for _, f := range filesChanged {
			s.TotalFilesChanged[f] = struct{}{}
		}
	} else {
		s.ConsecutiveNoChanges++
	}

	if err != "" {
		if err == s.LastError {
			s.ConsecutiveErrors++
		} else {
			s.ConsecutiveErrors = 1
		}
		s.LastError = err
	} else {
		s.ConsecutiveErrors = 0
		s.LastError = ""
	}
}

func (s *State) SetCurrentIterTokens(inputTokens, outputTokens int) {
	if s.CurrentIterTokens == nil {
		s.CurrentIterTokens = &ModelTokens{}
	}
	// Input seems cumulative from Claude, so replace
	s.CurrentIterTokens.InputTokens = inputTokens
	// Output is per-turn, so accumulate
	s.CurrentIterTokens.OutputTokens += outputTokens
}

func (s *State) FinalizeIterTokens(model string, inputTokens, outputTokens int) {
	if model != "" {
		s.Model = model
	}
	if s.Model == "" {
		return
	}
	if s.TokensByModel[s.Model] == nil {
		s.TokensByModel[s.Model] = &ModelTokens{}
	}
	s.TokensByModel[s.Model].InputTokens += inputTokens
	s.TokensByModel[s.Model].OutputTokens += outputTokens
	s.CurrentIterTokens = nil
}

func (s *State) TotalTokens() (input, output int) {
	for _, t := range s.TokensByModel {
		input += t.InputTokens
		output += t.OutputTokens
	}
	if s.CurrentIterTokens != nil {
		input += s.CurrentIterTokens.InputTokens
		output += s.CurrentIterTokens.OutputTokens
	}
	return
}

// RecordReviewIteration increments the review iteration counter.
func (s *State) RecordReviewIteration() {
	s.ReviewIterations++
}

// EnterReviewPhase marks that we've entered the review phase.
func (s *State) EnterReviewPhase() {
	s.InReviewPhase = true
}

// ExitReviewPhase marks that we've exited the review phase.
func (s *State) ExitReviewPhase() {
	s.InReviewPhase = false
}

type CheckResult struct {
	ShouldExit bool
	Reason     ExitReason
	Message    string
}

func Check(cfg Config, state *State) CheckResult {
	if state.Iteration > cfg.MaxIterations {
		return CheckResult{
			ShouldExit: true,
			Reason:     ExitReasonMaxIterations,
			Message:    "Maximum iterations reached",
		}
	}

	if state.ConsecutiveNoChanges >= cfg.StagnationLimit {
		return CheckResult{
			ShouldExit: true,
			Reason:     ExitReasonStagnation,
			Message:    "No file changes for multiple iterations",
		}
	}

	if state.ConsecutiveErrors >= 3 {
		return CheckResult{
			ShouldExit: true,
			Reason:     ExitReasonBlocked,
			Message:    "Repeated errors, blocking progress",
		}
	}

	if state.InReviewPhase && state.ReviewIterations >= cfg.MaxReviewIterations {
		return CheckResult{
			ShouldExit: true,
			Reason:     ExitReasonMaxReviewRetries,
			Message:    "Maximum review iterations reached with issues remaining",
		}
	}

	return CheckResult{ShouldExit: false}
}
