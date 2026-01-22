package review

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RunResult holds the result of a complete review run.
type RunResult struct {
	Passed       bool
	Iteration    int
	TotalIssues  int
	Results      []*ReviewResult
	Duration     time.Duration
	IssuesByPass map[string][]*ReviewResult
}

// HasCriticalIssues checks if any critical or high severity issues were found.
func (r *RunResult) HasCriticalIssues() bool {
	for _, result := range r.Results {
		for _, issue := range result.Issues {
			if issue.Severity == SeverityCritical || issue.Severity == SeverityHigh {
				return true
			}
		}
	}
	return false
}

// AllIssues returns all issues from all results.
func (r *RunResult) AllIssues() []Issue {
	var issues []Issue
	for _, result := range r.Results {
		issues = append(issues, result.Issues...)
	}
	return issues
}

// OutputCallback is called with progress messages.
type OutputCallback func(text string)

// Runner orchestrates the review process.
type Runner struct {
	config       Config
	agents       map[string]ReviewAgent
	agentsMu     sync.Mutex
	onOutput     OutputCallback
	agentFactory AgentFactory
}

// AgentFactory creates review agents from config.
type AgentFactory func(agentCfg Agent, defaultPrompt string) ReviewAgent

// NewRunner creates a new review runner.
func NewRunner(config Config, onOutput OutputCallback) *Runner {
	r := &Runner{
		config:   config,
		agents:   make(map[string]ReviewAgent),
		onOutput: onOutput,
	}
	r.agentFactory = r.defaultAgentFactory
	return r
}

// SetAgentFactory sets a custom agent factory (useful for testing).
func (r *Runner) SetAgentFactory(factory AgentFactory) {
	r.agentFactory = factory
}

// defaultAgentFactory creates ClaudeAgent instances.
func (r *Runner) defaultAgentFactory(agentCfg Agent, defaultPrompt string) ReviewAgent {
	prompt := defaultPrompt
	if agentCfg.Prompt != "" {
		prompt = agentCfg.Prompt
	}
	return NewClaudeAgent(agentCfg.Name, agentCfg.Focus, prompt)
}

// Run executes the full review pipeline.
// It runs all passes and returns the result. The caller (main loop) is responsible
// for iterating if issues are found.
func (r *Runner) Run(ctx context.Context, workingDir string, filesChanged []string) (*RunResult, error) {
	start := time.Now()

	result := &RunResult{
		Passed:       true,
		Iteration:    1,
		Results:      make([]*ReviewResult, 0),
		IssuesByPass: make(map[string][]*ReviewResult),
	}

	r.log("Running review passes")

	iterResults, err := r.runAllPasses(ctx, workingDir, filesChanged)
	if err != nil {
		result.Duration = time.Since(start)
		return result, err
	}

	result.Results = iterResults

	// Count issues
	issueCount := 0
	for _, res := range iterResults {
		issueCount += len(res.Issues)
	}
	result.TotalIssues = issueCount

	if issueCount == 0 {
		r.log("Review passed - no issues found")
		result.Passed = true
		result.Duration = time.Since(start)
		return result, nil
	}

	r.log(fmt.Sprintf("Found %d issues", issueCount))
	result.Passed = false
	result.Duration = time.Since(start)
	return result, nil
}

// runAllPasses executes all configured review passes.
func (r *Runner) runAllPasses(ctx context.Context, workingDir string, filesChanged []string) ([]*ReviewResult, error) {
	var allResults []*ReviewResult

	for _, pass := range r.config.Passes {
		r.log(fmt.Sprintf("Running pass: %s", pass.Name))

		passResults, err := r.runPass(ctx, pass, workingDir, filesChanged)
		if err != nil {
			return nil, fmt.Errorf("pass %s failed: %w", pass.Name, err)
		}

		allResults = append(allResults, passResults...)
	}

	return allResults, nil
}

// runPass executes a single review pass.
func (r *Runner) runPass(ctx context.Context, pass Pass, workingDir string, filesChanged []string) ([]*ReviewResult, error) {
	if pass.Parallel {
		return r.runAgentsParallel(ctx, pass.Agents, workingDir, filesChanged)
	}
	return r.runAgentsSequential(ctx, pass.Agents, workingDir, filesChanged)
}

// runAgentsParallel runs all agents in parallel.
func (r *Runner) runAgentsParallel(ctx context.Context, agents []Agent, workingDir string, filesChanged []string) ([]*ReviewResult, error) {
	var wg sync.WaitGroup
	results := make([]*ReviewResult, len(agents))
	errors := make([]error, len(agents))

	for i, agentCfg := range agents {
		wg.Add(1)
		go func(idx int, cfg Agent) {
			defer wg.Done()

			agent := r.getOrCreateAgent(cfg)
			r.log(fmt.Sprintf("  Running agent: %s", agent.Name()))

			result, err := agent.Review(ctx, workingDir, filesChanged)
			if err != nil {
				errors[idx] = err
				results[idx] = &ReviewResult{
					AgentName: cfg.Name,
					Error:     err,
				}
				return
			}

			results[idx] = result
			r.log(fmt.Sprintf("  Agent %s: %d issues found", agent.Name(), len(result.Issues)))
		}(i, agentCfg)
	}

	wg.Wait()

	// Check for any critical errors
	for _, err := range errors {
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return results, nil
}

// runAgentsSequential runs all agents sequentially.
func (r *Runner) runAgentsSequential(ctx context.Context, agents []Agent, workingDir string, filesChanged []string) ([]*ReviewResult, error) {
	results := make([]*ReviewResult, 0, len(agents))

	for _, agentCfg := range agents {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		agent := r.getOrCreateAgent(agentCfg)
		r.log(fmt.Sprintf("  Running agent: %s", agent.Name()))

		result, err := agent.Review(ctx, workingDir, filesChanged)
		if err != nil {
			result = &ReviewResult{
				AgentName: agentCfg.Name,
				Error:     err,
			}
		}

		results = append(results, result)
		r.log(fmt.Sprintf("  Agent %s: %d issues found", agent.Name(), len(result.Issues)))
	}

	return results, nil
}

// getOrCreateAgent gets a cached agent or creates a new one.
func (r *Runner) getOrCreateAgent(cfg Agent) ReviewAgent {
	r.agentsMu.Lock()
	defer r.agentsMu.Unlock()

	if agent, ok := r.agents[cfg.Name]; ok {
		return agent
	}

	defaultPrompt := GetDefaultPrompt(cfg.Name)
	agent := r.agentFactory(cfg, defaultPrompt)
	r.agents[cfg.Name] = agent

	return agent
}

// log outputs a message if callback is set.
func (r *Runner) log(message string) {
	if r.onOutput != nil {
		r.onOutput(fmt.Sprintf("[REVIEW] %s\n", message))
	}
}

// RegisterAgent registers a custom agent (useful for testing).
func (r *Runner) RegisterAgent(agent ReviewAgent) {
	r.agentsMu.Lock()
	defer r.agentsMu.Unlock()
	r.agents[agent.Name()] = agent
}
