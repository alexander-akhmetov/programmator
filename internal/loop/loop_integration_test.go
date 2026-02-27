package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitutil "github.com/alexander-akhmetov/programmator/internal/git"
	"github.com/alexander-akhmetov/programmator/internal/llm"
	"github.com/alexander-akhmetov/programmator/internal/plan"
	"github.com/alexander-akhmetov/programmator/internal/protocol"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"github.com/alexander-akhmetov/programmator/internal/safety"
	"github.com/alexander-akhmetov/programmator/internal/source"
)

// setupTestRepo creates a temp git repo with proper configuration for integration tests.
// The repo has user.name and user.email configured, and an initial commit so that
// setupGitWorkflow can succeed (branch creation, auto-commit, etc.).
//
// Returns:
//   - dir: the path to the temp directory (git repo root)
//   - cleanup: function to remove the temp directory
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "loop-integration-test-*")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(dir)
	}

	// Initialize git repo with go-git
	r, err := gogit.PlainInit(dir, false)
	require.NoError(t, err)

	// Configure git user for commits (required for auto-commit to work)
	cfg, err := r.Config()
	require.NoError(t, err)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@test.com"
	err = r.SetConfig(cfg)
	require.NoError(t, err)

	// Create initial commit (required for branch operations)
	readme := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(readme, []byte("# Test Repo\n"), 0644))

	wt, err := r.Worktree()
	require.NoError(t, err)

	_, err = wt.Add("README.md")
	require.NoError(t, err)

	_, err = wt.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@test.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	return dir, cleanup
}

// TestSetupTestRepo verifies that the helper creates a valid git repo
// with user config and initial commit, suitable for setupGitWorkflow.
func TestSetupTestRepo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Verify it's a valid git repo
	assert.True(t, gitutil.IsRepo(dir), "directory should be a git repo")

	// Verify we can open it via gitutil.NewRepo (used by setupGitWorkflow)
	repo, err := gitutil.NewRepo(dir)
	require.NoError(t, err)
	assert.Equal(t, dir, repo.WorkDir())

	// Verify README.md exists
	_, err = os.Stat(filepath.Join(dir, "README.md"))
	assert.NoError(t, err)

	// Verify there are no uncommitted changes (initial commit was made)
	hasChanges, err := repo.HasUncommittedChanges()
	require.NoError(t, err)
	assert.False(t, hasChanges)

	// Verify branch operations work (needed for setupGitWorkflow with AutoBranch)
	err = repo.CreateBranch("test-branch")
	require.NoError(t, err)

	current, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.Equal(t, "test-branch", current)
}

// planConfig holds configuration for creating a test plan file.
type planConfig struct {
	// Tasks is a list of task names to include in the plan.
	Tasks []string
	// ValidationCommands are commands to include in the validation section.
	ValidationCommands []string
	// CommitFiles commits the plan and working files to git after creation.
	// This is needed for tests that use AutoBranch since go-git checkout
	// may have issues with untracked files.
	CommitFiles bool
}

// writePlanFile creates a minimal plan file in the given directory.
// It also creates a working file (working.txt) that the fake invoker can modify.
//
// Returns:
//   - planPath: absolute path to the created plan file
//   - workingFilePath: absolute path to the working file
func writePlanFile(t *testing.T, dir string, cfg planConfig) (planPath, workingFilePath string) {
	t.Helper()

	if len(cfg.Tasks) == 0 {
		cfg.Tasks = []string{"Implement feature"}
	}

	// Build plan content
	var sb strings.Builder
	sb.WriteString("# Plan: Integration Test\n\n")

	// Add validation commands section if provided
	if len(cfg.ValidationCommands) > 0 {
		sb.WriteString("## Validation Commands\n")
		for _, cmd := range cfg.ValidationCommands {
			sb.WriteString("- `")
			sb.WriteString(cmd)
			sb.WriteString("`\n")
		}
		sb.WriteString("\n")
	}

	// Add tasks section
	sb.WriteString("## Tasks\n")
	for _, task := range cfg.Tasks {
		sb.WriteString("- [ ] ")
		sb.WriteString(task)
		sb.WriteString("\n")
	}

	planPath = filepath.Join(dir, "plan.md")
	err := os.WriteFile(planPath, []byte(sb.String()), 0644)
	require.NoError(t, err)

	// Create working file that the fake invoker can modify
	workingFilePath = filepath.Join(dir, "working.txt")
	err = os.WriteFile(workingFilePath, []byte("initial content\n"), 0644)
	require.NoError(t, err)

	// Optionally commit the files to git
	if cfg.CommitFiles {
		r, err := gogit.PlainOpen(dir)
		require.NoError(t, err)

		wt, err := r.Worktree()
		require.NoError(t, err)

		_, err = wt.Add("plan.md")
		require.NoError(t, err)
		_, err = wt.Add("working.txt")
		require.NoError(t, err)

		_, err = wt.Commit("Add plan and working files", &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@test.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
	}

	return planPath, workingFilePath
}

// TestWritePlanFile verifies that writePlanFile creates valid plan and working files.
func TestWritePlanFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	tests := []struct {
		name               string
		cfg                planConfig
		wantTasks          []string
		wantValidationCmds []string
	}{
		{
			name:      "default single task",
			cfg:       planConfig{},
			wantTasks: []string{"Implement feature"},
		},
		{
			name: "custom tasks",
			cfg: planConfig{
				Tasks: []string{"Task one", "Task two"},
			},
			wantTasks: []string{"Task one", "Task two"},
		},
		{
			name: "with validation commands",
			cfg: planConfig{
				Tasks:              []string{"Build project"},
				ValidationCommands: []string{"go build ./...", "go test ./..."},
			},
			wantTasks:          []string{"Build project"},
			wantValidationCmds: []string{"go build ./...", "go test ./..."},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a subdirectory for each test case
			subDir := filepath.Join(dir, tc.name)
			require.NoError(t, os.MkdirAll(subDir, 0755))

			planPath, workingFilePath := writePlanFile(t, subDir, tc.cfg)

			// Verify plan file exists and is parseable
			p, err := plan.ParseFile(planPath)
			require.NoError(t, err)
			assert.Equal(t, "Integration Test", p.Title)

			// Verify tasks
			require.Len(t, p.Tasks, len(tc.wantTasks))
			for i, wantTask := range tc.wantTasks {
				assert.Equal(t, wantTask, p.Tasks[i].Name)
				assert.False(t, p.Tasks[i].Completed, "task should start uncompleted")
			}

			// Verify validation commands
			assert.Equal(t, tc.wantValidationCmds, p.ValidationCommands)

			// Verify working file exists with initial content
			content, err := os.ReadFile(workingFilePath)
			require.NoError(t, err)
			assert.Equal(t, "initial content\n", string(content))
		})
	}
}

// sequenceResponse defines what the sequence invoker should return for a single invocation.
type sequenceResponse struct {
	// PhaseCompleted is the phase name to mark as completed (empty string for none).
	PhaseCompleted string
	// Status is the PROGRAMMATOR_STATUS value (CONTINUE, DONE, BLOCKED).
	Status protocol.Status
	// FilesChanged is the list of files to report as changed.
	FilesChanged []string
	// Summary is a brief description of what was done.
	Summary string
	// Error is set when status is BLOCKED.
	Error string
	// FileEdits maps file paths to the content to write (simulates Claude editing files).
	FileEdits map[string]string
}

// sequenceInvoker is a test double for llm.Invoker that returns deterministic responses
// in sequence and optionally edits files to simulate Claude making changes.
// This is more sophisticated than the simple fakeInvoker in loop_test.go, supporting
// multi-iteration scenarios with file modifications.
type sequenceInvoker struct {
	mu        sync.Mutex
	responses []sequenceResponse
	callIndex int
	calls     []sequenceInvokerCall
}

// sequenceInvokerCall records a single invocation for test assertions.
type sequenceInvokerCall struct {
	Prompt string
	Opts   llm.InvokeOptions
}

// newSequenceInvoker creates a sequence invoker with the given responses.
// Each call to Invoke() returns the next response in the sequence.
// If more calls are made than responses provided, it returns BLOCKED.
func newSequenceInvoker(responses []sequenceResponse) *sequenceInvoker {
	return &sequenceInvoker{
		responses: responses,
		calls:     make([]sequenceInvokerCall, 0),
	}
}

// Invoke implements llm.Invoker. It records the call, applies file edits,
// and returns the next predetermined response.
func (s *sequenceInvoker) Invoke(_ context.Context, prompt string, opts llm.InvokeOptions) (*llm.InvokeResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Record the call
	s.calls = append(s.calls, sequenceInvokerCall{
		Prompt: prompt,
		Opts:   opts,
	})

	// Get the response for this call
	var resp sequenceResponse
	if s.callIndex < len(s.responses) {
		resp = s.responses[s.callIndex]
	} else {
		// No more responses configured, return BLOCKED
		resp = sequenceResponse{
			Status:  protocol.StatusBlocked,
			Summary: "No more fake responses configured",
			Error:   "Exhausted fake responses",
		}
	}
	s.callIndex++

	// Apply file edits if any
	for filePath, content := range resp.FileEdits {
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("sequence invoker: failed to edit file %s: %w", filePath, err)
		}
	}

	// Build the response text with PROGRAMMATOR_STATUS block
	text := buildSequenceStatusBlock(resp)

	return &llm.InvokeResult{Text: text}, nil
}

// CallCount returns the number of times Invoke was called.
func (s *sequenceInvoker) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// Calls returns all recorded invocation calls.
func (s *sequenceInvoker) Calls() []sequenceInvokerCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]sequenceInvokerCall, len(s.calls))
	copy(result, s.calls)
	return result
}

// buildSequenceStatusBlock generates a PROGRAMMATOR_STATUS YAML block from a sequenceResponse.
func buildSequenceStatusBlock(resp sequenceResponse) string {
	var sb strings.Builder
	sb.WriteString("Some preamble text from Claude...\n\n")
	sb.WriteString("```\n")
	sb.WriteString(protocol.StatusBlockKey + ":\n")

	// phase_completed
	if resp.PhaseCompleted != "" {
		fmt.Fprintf(&sb, "  phase_completed: \"%s\"\n", resp.PhaseCompleted)
	} else {
		sb.WriteString("  phase_completed: null\n")
	}

	// status
	fmt.Fprintf(&sb, "  status: %s\n", resp.Status)

	// files_changed
	sb.WriteString("  files_changed:\n")
	if len(resp.FilesChanged) == 0 {
		sb.WriteString("    []\n")
	} else {
		for _, f := range resp.FilesChanged {
			fmt.Fprintf(&sb, "    - %s\n", f)
		}
	}

	// summary
	fmt.Fprintf(&sb, "  summary: \"%s\"\n", resp.Summary)

	// error (only if BLOCKED)
	if resp.Error != "" {
		fmt.Fprintf(&sb, "  error: \"%s\"\n", resp.Error)
	}

	sb.WriteString("```\n")
	return sb.String()
}

// TestSequenceInvoker verifies that the sequence invoker correctly simulates Claude responses.
func TestSequenceInvoker(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	workingFile := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(workingFile, []byte("original"), 0644))

	tests := []struct {
		name            string
		responses       []sequenceResponse
		expectedCalls   int
		checkFile       bool
		expectedContent string
	}{
		{
			name: "single CONTINUE response",
			responses: []sequenceResponse{
				{
					Status:         protocol.StatusContinue,
					PhaseCompleted: "Task 1",
					FilesChanged:   []string{"test.txt"},
					Summary:        "Completed task 1",
				},
			},
			expectedCalls: 1,
		},
		{
			name: "CONTINUE then DONE sequence",
			responses: []sequenceResponse{
				{
					Status:         protocol.StatusContinue,
					PhaseCompleted: "Task 1",
					FilesChanged:   []string{"file1.go"},
					Summary:        "Completed task 1",
				},
				{
					Status:         protocol.StatusDone,
					PhaseCompleted: "Task 2",
					FilesChanged:   []string{"file2.go"},
					Summary:        "All done",
				},
			},
			expectedCalls: 2,
		},
		{
			name: "with file edits",
			responses: []sequenceResponse{
				{
					Status:       protocol.StatusContinue,
					FilesChanged: []string{"test.txt"},
					Summary:      "Modified test.txt",
					FileEdits: map[string]string{
						workingFile: "modified content\n",
					},
				},
			},
			expectedCalls:   1,
			checkFile:       true,
			expectedContent: "modified content\n",
		},
		{
			name: "BLOCKED response with error",
			responses: []sequenceResponse{
				{
					Status:  protocol.StatusBlocked,
					Summary: "Could not proceed",
					Error:   "Missing dependency",
				},
			},
			expectedCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the working file for each test
			require.NoError(t, os.WriteFile(workingFile, []byte("original"), 0644))

			invoker := newSequenceInvoker(tc.responses)

			// Make all expected calls
			for i := 0; i < tc.expectedCalls; i++ {
				result, err := invoker.Invoke(context.Background(), "test prompt", llm.InvokeOptions{
					WorkingDir: dir,
				})
				require.NoError(t, err)
				require.NotNil(t, result)

				// Verify the response contains the status block
				assert.Contains(t, result.Text, protocol.StatusBlockKey)
				assert.Contains(t, result.Text, string(tc.responses[i].Status))
			}

			// Verify call count
			assert.Equal(t, tc.expectedCalls, invoker.CallCount())

			// Verify file content if requested
			if tc.checkFile {
				content, err := os.ReadFile(workingFile)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedContent, string(content))
			}

			// Verify all calls were recorded
			calls := invoker.Calls()
			assert.Len(t, calls, tc.expectedCalls)
			for _, call := range calls {
				assert.Equal(t, "test prompt", call.Prompt)
				assert.Equal(t, dir, call.Opts.WorkingDir)
			}
		})
	}
}

// TestSequenceInvokerExhaustsResponses verifies behavior when more calls are made than responses.
func TestSequenceInvokerExhaustsResponses(t *testing.T) {
	invoker := newSequenceInvoker([]sequenceResponse{
		{
			Status:  protocol.StatusContinue,
			Summary: "First response",
		},
	})

	// First call should succeed
	result1, err := invoker.Invoke(context.Background(), "prompt", llm.InvokeOptions{})
	require.NoError(t, err)
	assert.Contains(t, result1.Text, "CONTINUE")

	// Second call should return BLOCKED since we only provided one response
	result2, err := invoker.Invoke(context.Background(), "prompt", llm.InvokeOptions{})
	require.NoError(t, err)
	assert.Contains(t, result2.Text, "BLOCKED")
	assert.Contains(t, result2.Text, "Exhausted fake responses")
}

// TestCreateNoIssueReviewRunner verifies that the no-issue review runner helper
// returns a runner that passes reviews (reports no issues).
func TestCreateNoIssueReviewRunner(t *testing.T) {
	runner := createNoIssueReviewRunner(t)

	// Run a review iteration
	result, err := runner.RunIteration(context.Background(), t.TempDir(), []string{"test.go"})

	require.NoError(t, err)
	assert.True(t, result.Passed, "review should pass with no issues")
	assert.Equal(t, 0, result.TotalIssues, "should report zero issues")
}

// createNoIssueReviewRunner creates a review.Runner with a fake agent factory
// that returns no issues. This allows the loop to complete the review phase
// deterministically without calling external Claude.
//
// The runner is configured with a single test agent, and all agents created
// by the factory (including validators) return empty results.
func createNoIssueReviewRunner(t *testing.T) *review.Runner {
	t.Helper()

	cfg := review.Config{
		MaxIterations: 10,
		Parallel:      false, // Sequential for deterministic behavior
		Agents: []review.AgentConfig{
			{Name: "test_agent", Focus: []string{"integration test"}},
		},
	}

	runner := review.NewRunner(cfg)
	runner.SetAgentFactory(func(agentCfg review.AgentConfig, _ string) review.Agent {
		mock := review.NewMockAgent(agentCfg.Name)
		mock.SetReviewFunc(func(_ context.Context, _ string, _ []string) (*review.Result, error) {
			return &review.Result{
				AgentName: agentCfg.Name,
				Issues:    []review.Issue{}, // No issues - review passes
				Summary:   "No issues found",
			}, nil
		})
		return mock
	})

	return runner
}

// TestLoopRunWithPlanSource is the main integration test that runs Loop.Run
// with a PlanSource, fake invoker, and fake review runner to verify end-to-end
// behavior without external dependencies.
func TestLoopRunWithPlanSource(t *testing.T) {
	// Set up temp git repo and plan file
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks: []string{"Implement feature"},
	})

	// Configure sequence invoker to:
	// 1. First call: edit working.txt, complete the task, return CONTINUE
	// 2. (Loop will then run review, which passes with no issues)
	// 3. Loop should complete with ExitReasonComplete
	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusContinue,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified by fake Claude\n",
			},
		},
	})

	// Configure safety limits for testing
	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	// Create loop with fake invoker
	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)

	// Set up PlanSource
	src := source.NewPlanSource(planPath)
	loop.SetSource(src)

	// Set up fake review runner that always passes
	reviewRunner := createNoIssueReviewRunner(t)
	loop.SetReviewRunner(reviewRunner)

	// Configure review (required by loop validation)
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	})

	// Run the loop
	result, err := loop.Run(planPath)

	// Assert: no error
	require.NoError(t, err)
	require.NotNil(t, result)

	// Assert: exit reason is complete
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason,
		"expected complete exit reason, got %s", result.ExitReason)

	// Assert: plan checkbox updated to [x]
	updatedPlan, err := plan.ParseFile(planPath)
	require.NoError(t, err)
	require.Len(t, updatedPlan.Tasks, 1)
	assert.True(t, updatedPlan.Tasks[0].Completed,
		"task should be marked as completed in plan file")

	// Assert: TotalFilesChanged includes the edited file
	assert.Contains(t, result.TotalFilesChanged, "working.txt",
		"working.txt should be in TotalFilesChanged")

	// Assert: working file contains expected content
	content, err := os.ReadFile(workingFilePath)
	require.NoError(t, err)
	assert.Equal(t, "modified by fake Claude\n", string(content),
		"working file should contain modified content")

	// Assert: iteration count reflects the invocation
	// We expect 1 iteration for the task completion
	assert.GreaterOrEqual(t, result.Iterations, 1,
		"should have at least 1 iteration")

	// Assert: invoker was called at least once
	assert.GreaterOrEqual(t, invoker.CallCount(), 1,
		"invoker should have been called at least once")
}

// TestLoopRunWithTwoTaskPlan verifies the loop correctly handles a plan with
// two tasks, completing both phases in sequence and tracking all file changes
// across multiple iterations.
func TestLoopRunWithTwoTaskPlan(t *testing.T) {
	// Set up temp git repo and plan file with two tasks
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks: []string{"Task one: Setup", "Task two: Implementation"},
	})

	// Create a second working file for the second task
	workingFile2 := filepath.Join(dir, "feature.go")
	require.NoError(t, os.WriteFile(workingFile2, []byte("package main\n"), 0644))

	// Configure sequence invoker for two-task completion:
	// 1. First call: edit working.txt, complete Task one, return CONTINUE
	// 2. Second call: edit feature.go, complete Task two, return DONE
	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Task one: Setup",
			Status:         protocol.StatusContinue,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Completed setup task",
			FileEdits: map[string]string{
				workingFilePath: "setup complete\n",
			},
		},
		{
			PhaseCompleted: "Task two: Implementation",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"feature.go"},
			Summary:        "Completed implementation task",
			FileEdits: map[string]string{
				workingFile2: "package main\n\nfunc Feature() {}\n",
			},
		},
	})

	// Configure safety limits
	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	// Create loop with fake invoker
	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)

	// Set up PlanSource
	src := source.NewPlanSource(planPath)
	loop.SetSource(src)

	// Set up fake review runner that always passes
	reviewRunner := createNoIssueReviewRunner(t)
	loop.SetReviewRunner(reviewRunner)

	// Configure review
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents: []review.AgentConfig{
			{Name: "test_agent"},
		},
	})

	// Run the loop
	result, err := loop.Run(planPath)

	// Assert: no error
	require.NoError(t, err)
	require.NotNil(t, result)

	// Assert: exit reason is complete
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason,
		"expected complete exit reason, got %s", result.ExitReason)

	// Assert: both tasks are marked as completed in the plan file
	updatedPlan, err := plan.ParseFile(planPath)
	require.NoError(t, err)
	require.Len(t, updatedPlan.Tasks, 2, "plan should have 2 tasks")
	assert.True(t, updatedPlan.Tasks[0].Completed,
		"first task should be marked as completed")
	assert.True(t, updatedPlan.Tasks[1].Completed,
		"second task should be marked as completed")

	// Assert: TotalFilesChanged includes files from both phases
	assert.Contains(t, result.TotalFilesChanged, "working.txt",
		"working.txt should be in TotalFilesChanged")
	assert.Contains(t, result.TotalFilesChanged, "feature.go",
		"feature.go should be in TotalFilesChanged")

	// Assert: both working files contain expected content
	content1, err := os.ReadFile(workingFilePath)
	require.NoError(t, err)
	assert.Equal(t, "setup complete\n", string(content1),
		"first working file should contain modified content")

	content2, err := os.ReadFile(workingFile2)
	require.NoError(t, err)
	assert.Equal(t, "package main\n\nfunc Feature() {}\n", string(content2),
		"second working file should contain modified content")

	// Assert: invoker was called twice (once per task)
	assert.Equal(t, 2, invoker.CallCount(),
		"invoker should have been called exactly twice for two tasks")

	// Assert: iteration count reflects multiple invocations
	assert.GreaterOrEqual(t, result.Iterations, 2,
		"should have at least 2 iterations for two tasks")
}

// getCommitMessages returns commit messages from the git repo in reverse chronological order.
// It skips setup commits (initial commit and plan/working file commits from test helpers).
func getCommitMessages(t *testing.T, dir string) []string {
	t.Helper()

	r, err := gogit.PlainOpen(dir)
	require.NoError(t, err)

	ref, err := r.Head()
	require.NoError(t, err)

	iter, err := r.Log(&gogit.LogOptions{From: ref.Hash()})
	require.NoError(t, err)

	// Commits to skip (from test setup)
	skipMessages := map[string]bool{
		"Initial commit":             true,
		"Add plan and working files": true,
	}

	var messages []string
	err = iter.ForEach(func(c *object.Commit) error {
		msg := strings.TrimSpace(c.Message)
		if !skipMessages[msg] {
			messages = append(messages, msg)
		}
		return nil
	})
	require.NoError(t, err)

	return messages
}

// TestLoopRunWithAutoBranch verifies that the loop creates a new git branch
// when AutoBranch is enabled in GitWorkflowConfig.
func TestLoopRunWithAutoBranch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Implement feature"},
		CommitFiles: true, // Required for AutoBranch tests
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified content\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Enable AutoBranch
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		AutoBranch:   true,
		BranchPrefix: "test/",
	})

	// Record the starting branch
	repo, err := gitutil.NewRepo(dir)
	require.NoError(t, err)
	startBranch, err := repo.CurrentBranch()
	require.NoError(t, err)

	// Run the loop
	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: we should be on a new branch
	currentBranch, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.NotEqual(t, startBranch, currentBranch,
		"should have switched to a new branch")
	assert.True(t, strings.HasPrefix(currentBranch, "test/"),
		"branch should have the configured prefix, got: %s", currentBranch)
}

// TestLoopRunWithAutoCommit verifies that the loop creates git commits
// after each phase completion when AutoCommit is enabled.
func TestLoopRunWithAutoCommit(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Task one: Setup", "Task two: Implementation"},
		CommitFiles: true, // Required for AutoCommit tests
	})

	// Create second working file
	workingFile2 := filepath.Join(dir, "feature.go")
	require.NoError(t, os.WriteFile(workingFile2, []byte("package main\n"), 0644))

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Task one: Setup",
			Status:         protocol.StatusContinue,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Completed setup",
			FileEdits: map[string]string{
				workingFilePath: "setup complete\n",
			},
		},
		{
			PhaseCompleted: "Task two: Implementation",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"feature.go"},
			Summary:        "Completed implementation",
			FileEdits: map[string]string{
				workingFile2: "package main\n\nfunc Feature() {}\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Enable AutoCommit (but not AutoBranch)
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		AutoCommit: true,
	})

	// Run the loop
	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: commits were created for each phase
	messages := getCommitMessages(t, dir)
	require.Len(t, messages, 2, "should have 2 commits (one per phase)")

	// Commits are in reverse chronological order
	assert.Equal(t, "Task two: Implementation", messages[0],
		"second commit should be for Task two")
	assert.Equal(t, "Task one: Setup", messages[1],
		"first commit should be for Task one")

	// Note: plan.md will have uncommitted changes (checkbox updates)
	// because autoCommitPhase only commits files reported by Claude.
	// This is expected behavior - the plan file tracks progress but
	// isn't part of the "work" commits.
}

// TestLoopRunWithAutoBranchAndAutoCommit verifies that both AutoBranch
// and AutoCommit work together correctly.
func TestLoopRunWithAutoBranchAndAutoCommit(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Implement feature"},
		CommitFiles: true, // Required for AutoBranch/AutoCommit tests
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified content\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Enable both AutoBranch and AutoCommit
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		AutoBranch:   true,
		AutoCommit:   true,
		BranchPrefix: "feature/",
	})

	// Record the starting branch
	repo, err := gitutil.NewRepo(dir)
	require.NoError(t, err)
	startBranch, err := repo.CurrentBranch()
	require.NoError(t, err)

	// Run the loop
	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: we should be on a new branch
	currentBranch, err := repo.CurrentBranch()
	require.NoError(t, err)
	assert.NotEqual(t, startBranch, currentBranch,
		"should have switched to a new branch")
	assert.True(t, strings.HasPrefix(currentBranch, "feature/"),
		"branch should have the configured prefix, got: %s", currentBranch)

	// Assert: commit was created on the new branch
	messages := getCommitMessages(t, dir)
	require.Len(t, messages, 1, "should have 1 commit for the phase")
	assert.Equal(t, "Implement feature", messages[0])

	// Note: plan.md will have uncommitted changes (checkbox updates)
	// This is expected - the plan file tracks progress separately from work commits.
}

// TestLoopRunAutoCommitSkipsWhenNoFiles verifies that auto-commit is skipped
// when no files were changed in a phase (prevents empty commits).
func TestLoopRunAutoCommitSkipsWhenNoFiles(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Research task", "Implementation task"},
		CommitFiles: true, // Required for AutoCommit tests
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			// First phase: no files changed (research/investigation)
			PhaseCompleted: "Research task",
			Status:         protocol.StatusContinue,
			FilesChanged:   []string{}, // No files
			Summary:        "Completed research",
			// No FileEdits
		},
		{
			// Second phase: actual implementation
			PhaseCompleted: "Implementation task",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Completed implementation",
			FileEdits: map[string]string{
				workingFilePath: "implemented\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		AutoCommit: true,
	})

	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: only one commit (for the phase with files)
	messages := getCommitMessages(t, dir)
	require.Len(t, messages, 1, "should have only 1 commit (skipping phase with no files)")
	assert.Equal(t, "Implementation task", messages[0])
}

// TestLoopRunMoveCompletedPlan verifies that completed plan files are moved
// to the configured directory when MoveCompletedPlans is enabled.
func TestLoopRunMoveCompletedPlan(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Implement feature"},
		CommitFiles: true,
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified content\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Enable MoveCompletedPlans with default directory
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		MoveCompletedPlans: true,
	})

	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: plan file was moved to default completed directory
	expectedNewPath := filepath.Join(dir, "plans", "completed", "plan.md")
	assert.FileExists(t, expectedNewPath, "plan should be moved to plans/completed/")

	// Assert: original plan file no longer exists
	_, err = os.Stat(planPath)
	assert.True(t, os.IsNotExist(err), "original plan file should not exist")
}

// TestLoopRunMoveCompletedPlanCustomDir verifies that completed plan files
// are moved to a custom directory when CompletedPlansDir is configured.
func TestLoopRunMoveCompletedPlanCustomDir(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Implement feature"},
		CommitFiles: true,
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified content\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Enable MoveCompletedPlans with custom directory
	customDir := "archive/done"
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		MoveCompletedPlans: true,
		CompletedPlansDir:  customDir,
	})

	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: plan file was moved to custom directory
	expectedNewPath := filepath.Join(dir, customDir, "plan.md")
	assert.FileExists(t, expectedNewPath, "plan should be moved to custom directory")

	// Assert: original plan file no longer exists
	_, err = os.Stat(planPath)
	assert.True(t, os.IsNotExist(err), "original plan file should not exist")
}

// TestLoopRunMoveCompletedPlanWithAutoCommit verifies that the plan move
// is committed when both MoveCompletedPlans and AutoCommit are enabled.
func TestLoopRunMoveCompletedPlanWithAutoCommit(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Implement feature"},
		CommitFiles: true,
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified content\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// Enable both MoveCompletedPlans and AutoCommit
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		MoveCompletedPlans: true,
		AutoCommit:         true,
	})

	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: plan file was moved
	expectedNewPath := filepath.Join(dir, "plans", "completed", "plan.md")
	assert.FileExists(t, expectedNewPath, "plan should be moved to plans/completed/")

	// Assert: commits include both the phase commit and the move commit
	messages := getCommitMessages(t, dir)
	require.Len(t, messages, 2, "should have 2 commits (phase + move)")

	// Commits are in reverse chronological order
	assert.Equal(t, "chore: move completed plan to completed/", messages[0],
		"most recent commit should be the plan move")
	assert.Equal(t, "Implement feature", messages[1],
		"first commit should be for the phase")
}

// TestLoopRunMoveCompletedPlanDisabled verifies that plan files are NOT moved
// when MoveCompletedPlans is disabled (default behavior).
func TestLoopRunMoveCompletedPlanDisabled(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	planPath, workingFilePath := writePlanFile(t, dir, planConfig{
		Tasks:       []string{"Implement feature"},
		CommitFiles: true,
	})

	invoker := newSequenceInvoker([]sequenceResponse{
		{
			PhaseCompleted: "Implement feature",
			Status:         protocol.StatusDone,
			FilesChanged:   []string{"working.txt"},
			Summary:        "Implemented the feature",
			FileEdits: map[string]string{
				workingFilePath: "modified content\n",
			},
		},
	})

	safetyConfig := safety.Config{
		MaxIterations:       10,
		StagnationLimit:     3,
		Timeout:             60,
		MaxReviewIterations: 3,
	}

	loop := New(safetyConfig, dir, nil, false)
	loop.SetInvoker(invoker)
	loop.SetSource(source.NewPlanSource(planPath))
	loop.SetReviewRunner(createNoIssueReviewRunner(t))
	loop.SetReviewConfig(review.Config{
		MaxIterations: 3,
		Agents:        []review.AgentConfig{{Name: "test_agent"}},
	})

	// MoveCompletedPlans is NOT enabled (default)
	loop.SetGitWorkflowConfig(GitWorkflowConfig{
		MoveCompletedPlans: false,
	})

	result, err := loop.Run(planPath)

	require.NoError(t, err)
	assert.Equal(t, safety.ExitReasonComplete, result.ExitReason)

	// Assert: plan file still exists at original location
	assert.FileExists(t, planPath, "plan should remain at original location")

	// Assert: completed directory was NOT created
	completedDir := filepath.Join(dir, "plans", "completed")
	_, err = os.Stat(completedDir)
	assert.True(t, os.IsNotExist(err), "completed directory should not exist")
}
