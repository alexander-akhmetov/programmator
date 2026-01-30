package review

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// RunResult holds the result of a complete review run.
type RunResult struct {
	Passed       bool
	Iteration    int
	TotalIssues  int
	Results      []*Result
	Duration     time.Duration
	IssuesByPass map[string][]*Result
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
	total := 0
	for _, result := range r.Results {
		total += len(result.Issues)
	}
	issues := make([]Issue, 0, total)
	for _, result := range r.Results {
		issues = append(issues, result.Issues...)
	}
	return issues
}

// FilterBySeverity returns a new RunResult containing only issues matching the given severities.
// If severities is empty, returns a copy of the original result (all issues pass through).
func (r *RunResult) FilterBySeverity(severities []Severity) *RunResult {
	if len(severities) == 0 {
		// Empty filter = passthrough
		return r
	}

	// Build a set for fast lookup
	severitySet := make(map[Severity]struct{}, len(severities))
	for _, s := range severities {
		severitySet[s] = struct{}{}
	}

	filtered := &RunResult{
		Passed:       true,
		Iteration:    r.Iteration,
		TotalIssues:  0,
		Results:      make([]*Result, 0, len(r.Results)),
		IssuesByPass: make(map[string][]*Result),
		Duration:     r.Duration,
	}

	for _, result := range r.Results {
		filteredResult := &Result{
			AgentName:  result.AgentName,
			Issues:     make([]Issue, 0),
			Summary:    result.Summary,
			Error:      result.Error,
			Duration:   result.Duration,
			TokensUsed: result.TokensUsed,
		}

		for _, issue := range result.Issues {
			if _, ok := severitySet[issue.Severity]; ok {
				filteredResult.Issues = append(filteredResult.Issues, issue)
			}
		}

		filtered.Results = append(filtered.Results, filteredResult)
		filtered.TotalIssues += len(filteredResult.Issues)
	}

	filtered.Passed = filtered.TotalIssues == 0
	return filtered
}

// OutputCallback is called with progress messages.
type OutputCallback func(text string)

// Runner orchestrates the review process.
type Runner struct {
	config       Config
	agents       map[string]Agent
	agentsMu     sync.Mutex
	onOutput     OutputCallback
	agentFactory AgentFactory
}

// AgentFactory creates review agents from config.
type AgentFactory func(agentCfg AgentConfig, defaultPrompt string) Agent

// NewRunner creates a new review runner.
func NewRunner(config Config, onOutput OutputCallback) *Runner {
	r := &Runner{
		config:   config,
		agents:   make(map[string]Agent),
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
func (r *Runner) defaultAgentFactory(agentCfg AgentConfig, defaultPrompt string) Agent {
	prompt := defaultPrompt
	if agentCfg.Prompt != "" {
		prompt = agentCfg.Prompt
	}
	prompt = addTicketContext(prompt, r.config.TicketContext)
	var opts []ClaudeAgentOption
	if r.config.Timeout > 0 {
		opts = append(opts, WithTimeout(time.Duration(r.config.Timeout)*time.Second))
	}
	if r.config.ClaudeFlags != "" {
		opts = append(opts, WithClaudeArgs(strings.Fields(r.config.ClaudeFlags)))
	}
	if r.config.SettingsJSON != "" {
		opts = append(opts, WithSettingsJSON(r.config.SettingsJSON))
	}
	return NewClaudeAgent(agentCfg.Name, agentCfg.Focus, prompt, opts...)
}

func addTicketContext(prompt, ticketContext string) string {
	ticketContext = strings.TrimSpace(ticketContext)
	if ticketContext == "" {
		return prompt
	}

	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\n## Ticket Context (Full)\n")
	b.WriteString(ticketContext)
	b.WriteString("\n\n## Reviewer Role\n")
	b.WriteString("This code was implemented by another agent. Your job is to review the work only. ")
	b.WriteString("Do not implement changes or expand scope; report issues relative to the ticket requirements.")
	return b.String()
}

// runAgentsParallel runs all agents in parallel.
func (r *Runner) runAgentsParallel(ctx context.Context, agents []AgentConfig, workingDir string, filesChanged []string) ([]*Result, error) {
	var wg sync.WaitGroup
	results := make([]*Result, len(agents))
	errors := make([]error, len(agents))

	for i, agentCfg := range agents {
		wg.Add(1)
		go func(idx int, cfg AgentConfig) {
			defer wg.Done()

			agent := r.getOrCreateAgent(cfg)
			r.log(fmt.Sprintf("  Running agent: %s", agent.Name()))

			result, err := agent.Review(ctx, workingDir, filesChanged)
			if err != nil {
				errors[idx] = err
				results[idx] = &Result{
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

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Check for any agent errors
	for _, err := range errors {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// runAgentsSequential runs all agents sequentially.
func (r *Runner) runAgentsSequential(ctx context.Context, agents []AgentConfig, workingDir string, filesChanged []string) ([]*Result, error) {
	results := make([]*Result, 0, len(agents))

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
			result = &Result{
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
func (r *Runner) getOrCreateAgent(cfg AgentConfig) Agent {
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
func (r *Runner) RegisterAgent(agent Agent) {
	r.agentsMu.Lock()
	defer r.agentsMu.Unlock()
	r.agents[agent.Name()] = agent
}

// RunPhase executes a single phase and returns the result.
func (r *Runner) RunPhase(ctx context.Context, workingDir string, filesChanged []string, phase Phase) (*RunResult, error) {
	start := time.Now()

	result := &RunResult{
		Passed:       true,
		Iteration:    1,
		Results:      make([]*Result, 0),
		IssuesByPass: make(map[string][]*Result),
	}

	r.log(fmt.Sprintf("Running phase: %s", phase.Name))

	var passResults []*Result
	var err error

	if phase.Parallel {
		passResults, err = r.runAgentsParallel(ctx, phase.Agents, workingDir, filesChanged)
	} else {
		passResults, err = r.runAgentsSequential(ctx, phase.Agents, workingDir, filesChanged)
	}

	if err != nil {
		result.Duration = time.Since(start)
		return result, err
	}

	result.Results = passResults

	// Count issues
	issueCount := 0
	for _, res := range passResults {
		issueCount += len(res.Issues)
	}
	result.TotalIssues = issueCount
	result.Passed = issueCount == 0
	result.Duration = time.Since(start)

	return result, nil
}
