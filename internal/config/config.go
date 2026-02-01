// Package config provides unified configuration management for programmator.
// Configuration is loaded from multiple sources with the following precedence:
// embedded defaults → global file → env vars → local file → CLI flags
package config

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/worksonmyai/programmator/internal/review"
	"gopkg.in/yaml.v3"
)

//go:embed defaults/config.yaml
var defaultsFS embed.FS

// validModelName matches expected model name patterns (alphanumeric, dots, dashes, colons).
var validModelName = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

// ReviewPhase is deprecated. Kept only for migration from old configs.
// Use ReviewConfig.Agents instead.
type ReviewPhase struct {
	Name           string               `yaml:"name"`
	IterationLimit int                  `yaml:"iteration_limit,omitempty"`
	IterationPct   int                  `yaml:"iteration_pct,omitempty"`
	SeverityFilter []string             `yaml:"severity_filter,omitempty"`
	Agents         []review.AgentConfig `yaml:"agents"`
	Parallel       bool                 `yaml:"parallel"`
	Validate       bool                 `yaml:"validate,omitempty"`
}

// CodexConfig holds codex review configuration.
type CodexConfig struct {
	Command         string   `yaml:"command"`
	Model           string   `yaml:"model"`
	ReasoningEffort string   `yaml:"reasoning_effort"`
	TimeoutMs       int      `yaml:"timeout_ms"`
	Sandbox         string   `yaml:"sandbox"`
	ProjectDoc      string   `yaml:"project_doc"`
	ErrorPatterns   []string `yaml:"error_patterns"`

	// Set tracking for merge
	TimeoutMsSet bool `yaml:"-"`
}

// ReviewConfig holds review-specific configuration.
type ReviewConfig struct {
	MaxIterations int                  `yaml:"max_iterations"`
	Parallel      bool                 `yaml:"parallel"`
	Agents        []review.AgentConfig `yaml:"agents,omitempty"`

	// Deprecated: Phases is kept only for migration. Ignored at runtime when Agents is set.
	Phases []ReviewPhase `yaml:"phases,omitempty"`

	// Set tracking for merge
	MaxIterationsSet bool `yaml:"-"`
	ParallelSet      bool `yaml:"-"`
}

// GitConfig holds git workflow configuration.
type GitConfig struct {
	AutoCommit         bool   `yaml:"auto_commit"`          // Auto-commit after each phase completion
	MoveCompletedPlans bool   `yaml:"move_completed_plans"` // Move completed plans to completed/ directory
	CompletedPlansDir  string `yaml:"completed_plans_dir"`  // Directory for completed plans (default: plans/completed)
	BranchPrefix       string `yaml:"branch_prefix"`        // Prefix for auto-created branches (default: programmator/)

	// Set tracking for merge
	AutoCommitSet         bool `yaml:"-"`
	MoveCompletedPlansSet bool `yaml:"-"`
}

// Config holds all configuration settings for programmator.
// Fields ending in *Set track whether that field was explicitly set in config.
// This allows distinguishing explicit false/0 from "not set", enabling proper
// merge behavior where local config can override global config with zero values.
type Config struct {
	// Loop settings
	MaxIterations   int `yaml:"max_iterations"`
	StagnationLimit int `yaml:"stagnation_limit"`
	Timeout         int `yaml:"timeout"` // seconds

	// Claude settings
	ClaudeFlags     string `yaml:"claude_flags"`
	ClaudeConfigDir string `yaml:"claude_config_dir"`
	AnthropicAPIKey string `yaml:"anthropic_api_key"`

	// Ticket settings
	TicketCommand string `yaml:"ticket_command"` // Binary name for the ticket CLI (default: tk)

	// Progress log settings
	LogsDir string `yaml:"logs_dir"` // Directory for progress logs (default: ~/.programmator/logs)

	// Git workflow settings
	Git GitConfig `yaml:"git"`

	// Review settings (nested)
	Review ReviewConfig `yaml:"review"`

	// Codex review settings (nested)
	Codex CodexConfig `yaml:"codex"`

	// Prompts (loaded separately, not from YAML)
	Prompts *Prompts `yaml:"-"`

	// Set tracking for merge behavior
	MaxIterationsSet   bool `yaml:"-"`
	StagnationLimitSet bool `yaml:"-"`
	TimeoutSet         bool `yaml:"-"`

	// Private: track where config was loaded from
	configDir string
	localDir  string
	sources   []string // ordered list of sources that contributed to this config
}

// Source returns a human-readable description of where this config value came from.
func (c *Config) Sources() []string {
	return c.sources
}

// LocalDir returns the local project config directory if one was detected.
func (c *Config) LocalDir() string {
	return c.localDir
}

// ConfigDir returns the global config directory.
func (c *Config) ConfigDir() string {
	return c.configDir
}

// Load loads all configuration from the default locations.
// It auto-detects .programmator/ in the current working directory for local overrides.
// It installs defaults if needed.
func Load() (*Config, error) {
	globalDir := DefaultConfigDir()

	// Auto-detect local config directory in cwd
	var localDir string
	if cwd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(cwd, ".programmator")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			localDir = candidate
		}
	}

	return LoadWithDirs(globalDir, localDir)
}

// LoadWithDirs loads configuration with explicit global and local directories.
// Local config (.programmator/) overrides global config (~/.config/programmator/) per-field.
// If localDir is empty, only global config is used.
func LoadWithDirs(globalDir, localDir string) (*Config, error) {
	// Load in order: embedded → global → env → local
	// Each layer only overwrites fields that were explicitly set

	// 1. Start with embedded defaults
	cfg, err := loadEmbedded()
	if err != nil {
		return nil, fmt.Errorf("load embedded defaults: %w", err)
	}
	cfg.sources = append(cfg.sources, "embedded")

	// 2. Merge global config
	globalPath := filepath.Join(globalDir, "config.yaml")
	if globalCfg, err := loadFile(globalPath); err == nil {
		cfg.mergeFrom(globalCfg)
		cfg.sources = append(cfg.sources, globalPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("load global config: %w", err)
	}

	// 3. Apply environment variables (between global and local)
	cfg.applyEnv()

	// 4. Merge local config (highest file precedence)
	if localDir != "" {
		localPath := filepath.Join(localDir, "config.yaml")
		if localCfg, err := loadFile(localPath); err == nil {
			cfg.mergeFrom(localCfg)
			cfg.sources = append(cfg.sources, localPath)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load local config: %w", err)
		}
	}

	cfg.configDir = globalDir
	cfg.localDir = localDir

	// Load prompts with fallback chain
	prompts, err := LoadPrompts(globalDir, localDir)
	if err != nil {
		return nil, fmt.Errorf("load prompts: %w", err)
	}
	cfg.Prompts = prompts

	return cfg, nil
}

// DefaultConfigDir returns the default global configuration directory path.
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "programmator")
	}
	return filepath.Join(home, ".config", "programmator")
}

// loadEmbedded loads config from the embedded defaults.
func loadEmbedded() (*Config, error) {
	data, err := defaultsFS.ReadFile("defaults/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded defaults: %w", err)
	}
	return parseConfig(data)
}

// loadFile loads config from a file path.
func loadFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user's config file
	if err != nil {
		return nil, err
	}
	return parseConfigWithTracking(data)
}

// parseConfig parses YAML config data into a Config struct.
func parseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// parseConfigWithTracking parses YAML config and tracks which fields were set.
func parseConfigWithTracking(data []byte) (*Config, error) {
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, err
	}

	// Parse into a map to detect which fields were explicitly set
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Track top-level fields
	if _, ok := raw["max_iterations"]; ok {
		cfg.MaxIterationsSet = true
	}
	if _, ok := raw["stagnation_limit"]; ok {
		cfg.StagnationLimitSet = true
	}
	if _, ok := raw["timeout"]; ok {
		cfg.TimeoutSet = true
	}

	// Track review fields
	if review, ok := raw["review"].(map[string]any); ok {
		// Silently ignore legacy "passes" key; users should migrate to "agents".
		delete(review, "passes")
		if _, ok := review["max_iterations"]; ok {
			cfg.Review.MaxIterationsSet = true
		}
		if _, ok := review["parallel"]; ok {
			cfg.Review.ParallelSet = true
		}
	}

	// Track codex fields
	if codex, ok := raw["codex"].(map[string]any); ok {
		if _, ok := codex["timeout_ms"]; ok {
			cfg.Codex.TimeoutMsSet = true
		}
	}

	// Track git fields
	if git, ok := raw["git"].(map[string]any); ok {
		if _, ok := git["auto_commit"]; ok {
			cfg.Git.AutoCommitSet = true
		}
		if _, ok := git["move_completed_plans"]; ok {
			cfg.Git.MoveCompletedPlansSet = true
		}
	}

	return cfg, nil
}

// applyEnv applies environment variables to the config.
// Env vars sit between global and local config in precedence.
func (c *Config) applyEnv() {
	if v := os.Getenv("PROGRAMMATOR_MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxIterations = n
			c.MaxIterationsSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_MAX_ITERATIONS")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_MAX_ITERATIONS=%q: %v", v, err)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_STAGNATION_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.StagnationLimit = n
			c.StagnationLimitSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_STAGNATION_LIMIT")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_STAGNATION_LIMIT=%q: %v", v, err)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Timeout = n
			c.TimeoutSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_TIMEOUT")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_TIMEOUT=%q: %v", v, err)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CLAUDE_FLAGS"); v != "" {
		c.ClaudeFlags = v
		c.sources = append(c.sources, "env:PROGRAMMATOR_CLAUDE_FLAGS")
	}

	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		c.ClaudeConfigDir = v
		c.sources = append(c.sources, "env:CLAUDE_CONFIG_DIR")
	}

	if v := os.Getenv("PROGRAMMATOR_ANTHROPIC_API_KEY"); v != "" {
		c.AnthropicAPIKey = v
		c.sources = append(c.sources, "env:PROGRAMMATOR_ANTHROPIC_API_KEY")
	}

	if v := os.Getenv("PROGRAMMATOR_TICKET_COMMAND"); v != "" {
		c.TicketCommand = v
		c.sources = append(c.sources, "env:PROGRAMMATOR_TICKET_COMMAND")
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_COMMAND"); v != "" {
		if isValidCommandName(v) {
			c.Codex.Command = v
			c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_COMMAND")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_CODEX_COMMAND=%q: must be a simple binary name", v)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_MODEL"); v != "" {
		if validModelName.MatchString(v) {
			c.Codex.Model = v
			c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_MODEL")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_CODEX_MODEL=%q: must match [a-zA-Z0-9._:-]+", v)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_REASONING_EFFORT"); v != "" {
		if isValidReasoningEffort(v) {
			c.Codex.ReasoningEffort = v
			c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_REASONING_EFFORT")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_CODEX_REASONING_EFFORT=%q: must be one of low, medium, high, xhigh", v)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Codex.TimeoutMs = n
			c.Codex.TimeoutMsSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_TIMEOUT_MS")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_CODEX_TIMEOUT_MS=%q: %v", v, err)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_SANDBOX"); v != "" {
		if isValidSandboxMode(v) {
			c.Codex.Sandbox = v
			c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_SANDBOX")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_CODEX_SANDBOX=%q: must be one of read-only, network, off", v)
		}
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_PROJECT_DOC"); v != "" {
		c.Codex.ProjectDoc = v
		c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_PROJECT_DOC")
	}

	if v := os.Getenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS"); v != "" {
		parts := strings.Split(v, ",")
		var cleaned []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cleaned = append(cleaned, p)
			}
		}
		c.Codex.ErrorPatterns = cleaned
		c.sources = append(c.sources, "env:PROGRAMMATOR_CODEX_ERROR_PATTERNS")
	}

	if v := os.Getenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Review.MaxIterations = n
			c.Review.MaxIterationsSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_MAX_REVIEW_ITERATIONS")
		} else {
			log.Printf("warning: ignoring invalid PROGRAMMATOR_MAX_REVIEW_ITERATIONS=%q: %v", v, err)
		}
	}
}

// validReasoningEfforts are the accepted reasoning effort levels for codex.
var validReasoningEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true,
}

// isValidReasoningEffort checks that the value is a known reasoning effort level.
func isValidReasoningEffort(v string) bool {
	return validReasoningEfforts[v]
}

// validSandboxModes are the accepted sandbox modes for codex.
var validSandboxModes = map[string]bool{
	"read-only": true, "network": true, "off": true,
}

// isValidSandboxMode checks that the value is a known sandbox mode.
func isValidSandboxMode(v string) bool {
	return validSandboxModes[v]
}

// isValidCommandName checks that a command name contains only safe characters.
// Rejects names with path separators, shell metacharacters, or whitespace.
func isValidCommandName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

// mergeFrom merges non-empty/set values from src into c.
func (c *Config) mergeFrom(src *Config) {
	if src.MaxIterationsSet {
		c.MaxIterations = src.MaxIterations
		c.MaxIterationsSet = true
	}
	if src.StagnationLimitSet {
		c.StagnationLimit = src.StagnationLimit
		c.StagnationLimitSet = true
	}
	if src.TimeoutSet {
		c.Timeout = src.Timeout
		c.TimeoutSet = true
	}
	if src.ClaudeFlags != "" {
		c.ClaudeFlags = src.ClaudeFlags
	}
	if src.ClaudeConfigDir != "" {
		c.ClaudeConfigDir = src.ClaudeConfigDir
	}
	if src.AnthropicAPIKey != "" {
		log.Printf("warning: anthropic_api_key loaded from config file — ensure this is a trusted source")
		c.AnthropicAPIKey = src.AnthropicAPIKey
	}
	if src.LogsDir != "" {
		c.LogsDir = src.LogsDir
	}
	if src.TicketCommand != "" {
		c.TicketCommand = src.TicketCommand
	}

	// Review config merge
	if src.Review.MaxIterationsSet {
		c.Review.MaxIterations = src.Review.MaxIterations
		c.Review.MaxIterationsSet = true
	}
	if src.Review.ParallelSet {
		c.Review.Parallel = src.Review.Parallel
		c.Review.ParallelSet = true
	}
	if len(src.Review.Agents) > 0 {
		c.Review.Agents = src.Review.Agents
	}
	if len(src.Review.Phases) > 0 {
		c.Review.Phases = src.Review.Phases
	}

	if src.Codex.Command != "" {
		if isValidCommandName(src.Codex.Command) {
			c.Codex.Command = src.Codex.Command
		} else {
			log.Printf("warning: ignoring invalid codex.command=%q from config: must be a simple binary name", src.Codex.Command)
		}
	}
	if src.Codex.Model != "" {
		if validModelName.MatchString(src.Codex.Model) {
			c.Codex.Model = src.Codex.Model
		} else {
			log.Printf("warning: ignoring invalid codex.model=%q from config: must match [a-zA-Z0-9._:-]+", src.Codex.Model)
		}
	}
	if src.Codex.ReasoningEffort != "" {
		if isValidReasoningEffort(src.Codex.ReasoningEffort) {
			c.Codex.ReasoningEffort = src.Codex.ReasoningEffort
		} else {
			log.Printf("warning: ignoring invalid codex.reasoning_effort=%q from config: must be one of low, medium, high, xhigh", src.Codex.ReasoningEffort)
		}
	}
	if src.Codex.TimeoutMsSet {
		c.Codex.TimeoutMs = src.Codex.TimeoutMs
		c.Codex.TimeoutMsSet = true
	}
	if src.Codex.Sandbox != "" {
		if isValidSandboxMode(src.Codex.Sandbox) {
			c.Codex.Sandbox = src.Codex.Sandbox
		} else {
			log.Printf("warning: ignoring invalid codex.sandbox=%q from config: must be one of read-only, network, off", src.Codex.Sandbox)
		}
	}
	if src.Codex.ProjectDoc != "" {
		c.Codex.ProjectDoc = src.Codex.ProjectDoc
	}
	if len(src.Codex.ErrorPatterns) > 0 {
		c.Codex.ErrorPatterns = src.Codex.ErrorPatterns
	}

	// Git config merge
	if src.Git.AutoCommitSet {
		c.Git.AutoCommit = src.Git.AutoCommit
		c.Git.AutoCommitSet = true
	}
	if src.Git.MoveCompletedPlansSet {
		c.Git.MoveCompletedPlans = src.Git.MoveCompletedPlans
		c.Git.MoveCompletedPlansSet = true
	}
	if src.Git.CompletedPlansDir != "" {
		c.Git.CompletedPlansDir = src.Git.CompletedPlansDir
	}
	if src.Git.BranchPrefix != "" {
		c.Git.BranchPrefix = src.Git.BranchPrefix
	}
}

// ApplyCLIFlags applies CLI flag overrides to the config.
// CLI flags have the highest precedence.
func (c *Config) ApplyCLIFlags(maxIterations, stagnationLimit, timeout int) {
	if maxIterations > 0 {
		c.MaxIterations = maxIterations
		c.MaxIterationsSet = true
		c.sources = append(c.sources, "cli:max-iterations")
	}
	if stagnationLimit > 0 {
		c.StagnationLimit = stagnationLimit
		c.StagnationLimitSet = true
		c.sources = append(c.sources, "cli:stagnation-limit")
	}
	if timeout > 0 {
		c.Timeout = timeout
		c.TimeoutSet = true
		c.sources = append(c.sources, "cli:timeout")
	}
}
