package review

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/worksonmyai/programmator/internal/event"
	"github.com/worksonmyai/programmator/internal/protocol"
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
	onEvent      event.Handler
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
	errs := make([]error, len(agents))

	for i, agentCfg := range agents {
		wg.Add(1)
		go func(idx int, cfg AgentConfig) {
			defer wg.Done()

			agent := r.getOrCreateAgent(cfg)
			r.log(fmt.Sprintf("  Running agent: %s", agent.Name()))

			result, err := agent.Review(ctx, workingDir, filesChanged)
			if err != nil {
				errs[idx] = err
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

	// Check for any agent errors (aggregate all failures)
	var agentErrs []error
	for i, err := range errs {
		if err != nil {
			agentErrs = append(agentErrs, fmt.Errorf("agent %s: %w", agents[i].Name, err))
		}
	}
	if len(agentErrs) > 0 {
		return nil, fmt.Errorf("agent errors: %w", errors.Join(agentErrs...))
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

// SetEventCallback sets the typed event handler for review events.
func (r *Runner) SetEventCallback(cb event.Handler) {
	r.onEvent = cb
}

// log outputs a message if callback is set.
func (r *Runner) log(message string) {
	if r.onEvent != nil {
		r.onEvent(event.Review(message))
	}
	if r.onOutput != nil && r.onEvent == nil {
		r.onOutput(fmt.Sprintf(protocol.MarkerReview+" %s\n", message))
	}
}

// RegisterAgent registers a custom agent (useful for testing).
func (r *Runner) RegisterAgent(agent Agent) {
	r.agentsMu.Lock()
	defer r.agentsMu.Unlock()
	r.agents[agent.Name()] = agent
}

// ValidateSimplifications runs a validation agent to filter simplification findings.
// It returns the validated result, or the original result if validation fails.
func (r *Runner) ValidateSimplifications(ctx context.Context, workingDir string, simplificationResult *Result) (*Result, error) {
	if len(simplificationResult.Issues) == 0 {
		return simplificationResult, nil
	}

	r.log("Validating simplification suggestions...")

	// Format the original findings as input for the validator
	input := FormatIssuesMarkdown([]*Result{simplificationResult})

	validatorCfg := AgentConfig{
		Name:  "simplification-validator",
		Focus: []string{"filter low-value simplification suggestions"},
	}

	agent := r.getOrCreateAgent(validatorCfg)

	// Build a custom prompt with the simplification findings
	result, err := agent.Review(ctx, workingDir, []string{"SIMPLIFICATION_INPUT:\n" + input})
	if err != nil {
		r.log(fmt.Sprintf("Simplification validation failed, using original results: %v", err))
		return simplificationResult, nil
	}

	if result == nil {
		r.log("Simplification validator returned no output, filtering all suggestions")
		return &Result{
			AgentName: "simplification",
			Issues:    []Issue{},
			Summary:   "All simplification suggestions filtered by validator",
		}, nil
	}

	if len(result.Issues) == 0 {
		r.log("Simplification validator filtered all suggestions")
		return &Result{
			AgentName: "simplification",
			Issues:    []Issue{},
			Summary:   "All simplification suggestions filtered by validator",
			Duration:  result.Duration,
		}, nil
	}

	// Return validated results, keeping the original agent name
	result.AgentName = "simplification"
	r.log(fmt.Sprintf("Simplification validator kept %d of %d suggestions", len(result.Issues), len(simplificationResult.Issues)))
	return result, nil
}

// ValidateIssues runs a validation agent to filter false positives from all review results
// (excluding simplification, which has its own validator).
// It returns updated results with filtered issues, or the original results if validation fails.
func (r *Runner) ValidateIssues(ctx context.Context, workingDir string, results []*Result) ([]*Result, error) {
	// Collect non-simplification results that have issues
	var toValidate []*Result
	for _, res := range results {
		if res.AgentName == "simplification" {
			continue
		}
		if len(res.Issues) > 0 {
			toValidate = append(toValidate, res)
		}
	}

	if len(toValidate) == 0 {
		return results, nil
	}

	r.log("Validating issues across agents...")

	input := FormatIssuesYAML(toValidate)

	validatorCfg := AgentConfig{
		Name:  "issue-validator",
		Focus: []string{"filter false positive review findings"},
	}

	agent := r.getOrCreateAgent(validatorCfg)

	// VALIDATION_INPUT prefix signals to the validator agent that the filesChanged
	// content is structured YAML input to validate, not actual file paths.
	validatorResult, err := agent.Review(ctx, workingDir, []string{"VALIDATION_INPUT:\n" + input})
	if err != nil {
		r.log(fmt.Sprintf("Issue validation failed, using original results: %v", err))
		return results, nil
	}

	// Nil result with no error means validator produced no usable output; keep originals.
	if validatorResult == nil {
		r.log("Issue validator returned no result, using original results")
		return results, nil
	}
	if strings.TrimSpace(validatorResult.Summary) == noStructuredReviewOutputSummary {
		r.log("Issue validator returned no structured output, using original results")
		return results, nil
	}

	// Build verdict map from validator output (id → verdict string).
	verdicts := make(map[string]string)
	for _, issue := range validatorResult.Issues {
		if issue.ID != "" {
			verdicts[issue.ID] = strings.ToLower(strings.TrimSpace(issue.Verdict))
		}
	}

	// If validator returned issues but none had IDs, assume it didn't understand
	// the ID protocol and fall back to originals (safe default: keep all).
	if len(validatorResult.Issues) > 0 && len(verdicts) == 0 {
		r.log("Issue validator returned issues without IDs, using original results")
		return results, nil
	}

	// Filter original results by verdict.
	// - "false_positive" → drop
	// - "valid" → keep
	// - no verdict / unknown verdict / ID not in validator output → keep (safe default)
	totalBefore := 0
	totalAfter := 0
	filtered := make([]*Result, len(results))
	for i, res := range results {
		if res.AgentName == "simplification" || len(res.Issues) == 0 {
			filtered[i] = res
			continue
		}

		totalBefore += len(res.Issues)
		kept := make([]Issue, 0, len(res.Issues))
		for _, issue := range res.Issues {
			verdict, hasVerdict := verdicts[issue.ID]
			if hasVerdict && verdict == "false_positive" {
				continue
			}
			kept = append(kept, issue)
		}
		totalAfter += len(kept)

		filtered[i] = &Result{
			AgentName:  res.AgentName,
			Issues:     kept,
			Summary:    res.Summary,
			Error:      res.Error,
			Duration:   res.Duration,
			TokensUsed: res.TokensUsed,
		}
	}

	r.log(fmt.Sprintf("Issue validator kept %d of %d issues", totalAfter, totalBefore))
	return filtered, nil
}

// assignIssueIDs assigns stable IDs to issues that don't already have one.
func assignIssueIDs(results []*Result) {
	for _, res := range results {
		for i := range res.Issues {
			if res.Issues[i].ID == "" {
				res.Issues[i].ID = issueFingerprint(res.AgentName, res.Issues[i])
			}
		}
	}
}

// issueFingerprint generates a deterministic ID from agent name and issue fields.
func issueFingerprint(agent string, issue Issue) string {
	desc := strings.ToLower(strings.TrimSpace(issue.Description))
	cat := strings.ToLower(issue.Category)
	linePart := ""
	if issue.Line > 0 {
		linePart = fmt.Sprintf("%d", issue.Line)
	}
	data := fmt.Sprintf("%s|%s|%s|%s|%s", agent, issue.File, linePart, cat, desc)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash[:8])
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

	// Assign stable IDs to issues for tracking across iterations
	assignIssueIDs(passResults)

	// Post-process: validate simplification results
	for i, res := range passResults {
		if res.AgentName == "simplification" && len(res.Issues) > 0 {
			validated, validateErr := r.ValidateSimplifications(ctx, workingDir, res)
			if validateErr == nil {
				passResults[i] = validated
				// Re-assign IDs to validated simplification issues since the validator
				// returns new Issue structs that may lack IDs.
				assignIssueIDs([]*Result{passResults[i]})
			}
		}
	}

	// Post-process: validate all issues (excluding simplification)
	if phase.Validate {
		totalNonSimp := 0
		for _, res := range passResults {
			if res.AgentName != "simplification" {
				totalNonSimp += len(res.Issues)
			}
		}
		if totalNonSimp > 0 {
			validated, validateErr := r.ValidateIssues(ctx, workingDir, passResults)
			if validateErr == nil {
				passResults = validated
			}
		}
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
