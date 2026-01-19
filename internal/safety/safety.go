// Package safety implements exit condition checks for the main loop.
package safety

import (
	"os"
	"strconv"
)

const (
	DefaultMaxIterations   = 50
	DefaultStagnationLimit = 3
	DefaultTimeout         = 900 // seconds
)

type ExitReason string

const (
	ExitReasonComplete      ExitReason = "complete"
	ExitReasonMaxIterations ExitReason = "max_iterations"
	ExitReasonStagnation    ExitReason = "stagnation"
	ExitReasonBlocked       ExitReason = "blocked"
	ExitReasonError         ExitReason = "error"
	ExitReasonUserInterrupt ExitReason = "user_interrupt"
)

type Config struct {
	MaxIterations   int
	StagnationLimit int
	Timeout         int
	ClaudeFlags     string
}

func ConfigFromEnv() Config {
	cfg := Config{
		MaxIterations:   DefaultMaxIterations,
		StagnationLimit: DefaultStagnationLimit,
		Timeout:         DefaultTimeout,
		ClaudeFlags:     "--dangerously-skip-permissions",
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

	return cfg
}

type State struct {
	Iteration            int
	ConsecutiveNoChanges int
	LastError            string
	ConsecutiveErrors    int
	FilesChangedHistory  [][]string
	TotalFilesChanged    map[string]struct{}
}

func NewState() *State {
	return &State{
		FilesChangedHistory: make([][]string, 0),
		TotalFilesChanged:   make(map[string]struct{}),
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

	return CheckResult{ShouldExit: false}
}
