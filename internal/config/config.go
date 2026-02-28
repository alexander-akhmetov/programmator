// Package config provides unified configuration management for programmator.
// Configuration is loaded from multiple sources with the following precedence:
// embedded defaults → global file → local file → CLI flags
package config

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/alexander-akhmetov/programmator/internal/dirs"
	"github.com/alexander-akhmetov/programmator/internal/review"
	"gopkg.in/yaml.v3"
)

//go:embed defaults/config.yaml
var defaultsFS embed.FS

// validExecutors is the set of supported executor names.
var validExecutors = map[string]bool{
	"claude":   true,
	"pi":       true,
	"opencode": true,
	"codex":    true,
	"":         true, // empty defaults to "claude"
}

// ClaudeConfig holds Claude executor configuration.
type ClaudeConfig struct {
	Flags           string `yaml:"flags"`
	ConfigDir       string `yaml:"config_dir"`
	AnthropicAPIKey string `yaml:"anthropic_api_key"`
}

// PiConfig holds pi coding agent executor configuration.
type PiConfig struct {
	Flags     string `yaml:"flags"`
	ConfigDir string `yaml:"config_dir"`
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key"`
}

// OpenCodeConfig holds OpenCode executor configuration.
type OpenCodeConfig struct {
	Flags     string `yaml:"flags"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key"`
	ConfigDir string `yaml:"config_dir"`
}

// CodexConfig holds Codex executor configuration.
type CodexConfig struct {
	Flags  string `yaml:"flags"`
	Model  string `yaml:"model"`
	APIKey string `yaml:"api_key"`
}

// ReviewExecutorConfig holds review-specific executor overrides.
type ReviewExecutorConfig struct {
	Name     string         `yaml:"name"`
	Claude   ClaudeConfig   `yaml:"claude"`
	Pi       PiConfig       `yaml:"pi"`
	OpenCode OpenCodeConfig `yaml:"opencode"`
	Codex    CodexConfig    `yaml:"codex"`
}

// ReviewValidatorsConfig controls validation passes that run after review agents within each iteration.
type ReviewValidatorsConfig struct {
	Issue          bool `yaml:"issue"`
	Simplification bool `yaml:"simplification"`
}

// ReviewConfig holds review-specific configuration.
type ReviewConfig struct {
	MaxIterations int                    `yaml:"max_iterations"`
	Parallel      bool                   `yaml:"parallel"`
	Executor      ReviewExecutorConfig   `yaml:"executor,omitempty"`
	Include       []string               `yaml:"include,omitempty"`
	Exclude       []string               `yaml:"exclude,omitempty"`
	Overrides     []review.AgentConfig   `yaml:"overrides,omitempty"`
	Agents        []review.AgentConfig   `yaml:"agents,omitempty"`
	Validators    ReviewValidatorsConfig `yaml:"validators"`
}

// GitConfig holds git workflow configuration.
type GitConfig struct {
	AutoCommit         bool   `yaml:"auto_commit"`
	MoveCompletedPlans bool   `yaml:"move_completed_plans"`
	CompletedPlansDir  string `yaml:"completed_plans_dir"`
	BranchPrefix       string `yaml:"branch_prefix"`
}

// Config holds all configuration settings for programmator.
type Config struct {
	MaxIterations   int `yaml:"max_iterations"`
	StagnationLimit int `yaml:"stagnation_limit"`
	Timeout         int `yaml:"timeout"` // seconds

	Executor      string         `yaml:"executor"`
	Claude        ClaudeConfig   `yaml:"claude"`
	Pi            PiConfig       `yaml:"pi"`
	OpenCode      OpenCodeConfig `yaml:"opencode"`
	Codex         CodexConfig    `yaml:"codex"`
	TicketCommand string         `yaml:"ticket_command"`

	Git    GitConfig    `yaml:"git"`
	Review ReviewConfig `yaml:"review"`

	// Prompts (loaded separately, not from YAML)
	Prompts *Prompts `yaml:"-"`

	// Private: track where config was loaded from
	configDir string
	localDir  string
	sources   []string
}

// configOverlay is used for parsing override YAML files.
// Pointer types distinguish "not set" (nil) from "explicitly set to zero/false".
type configOverlay struct {
	MaxIterations   *int           `yaml:"max_iterations"`
	StagnationLimit *int           `yaml:"stagnation_limit"`
	Timeout         *int           `yaml:"timeout"`
	Executor        string         `yaml:"executor"`
	Claude          ClaudeConfig   `yaml:"claude"`
	Pi              PiConfig       `yaml:"pi"`
	OpenCode        OpenCodeConfig `yaml:"opencode"`
	Codex           CodexConfig    `yaml:"codex"`
	TicketCommand   string         `yaml:"ticket_command"`

	Git    gitOverlay    `yaml:"git"`
	Review reviewOverlay `yaml:"review"`
}

type reviewOverlay struct {
	MaxIterations *int                    `yaml:"max_iterations"`
	Parallel      *bool                   `yaml:"parallel"`
	Executor      *ReviewExecutorConfig   `yaml:"executor,omitempty"`
	Include       []string                `yaml:"include,omitempty"`
	Exclude       []string                `yaml:"exclude,omitempty"`
	Overrides     []review.AgentConfig    `yaml:"overrides,omitempty"`
	Agents        []review.AgentConfig    `yaml:"agents,omitempty"`
	Validators    reviewValidatorsOverlay `yaml:"validators,omitempty"`
}

type reviewValidatorsOverlay struct {
	Issue          *bool `yaml:"issue"`
	Simplification *bool `yaml:"simplification"`
}

type gitOverlay struct {
	AutoCommit         *bool  `yaml:"auto_commit"`
	MoveCompletedPlans *bool  `yaml:"move_completed_plans"`
	CompletedPlansDir  string `yaml:"completed_plans_dir"`
	BranchPrefix       string `yaml:"branch_prefix"`
}

// Sources returns a human-readable description of where config values came from.
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

// Validate checks the configuration for invalid values.
func (c *Config) Validate() error {
	if !validExecutors[c.Executor] {
		return fmt.Errorf("unknown executor %q (supported: claude, pi, opencode, codex)", c.Executor)
	}
	if c.Review.Executor.Name != "" && !validExecutors[c.Review.Executor.Name] {
		return fmt.Errorf("unknown review.executor.name %q (supported: claude, pi, opencode, codex)", c.Review.Executor.Name)
	}
	return nil
}

// Load loads all configuration from the default locations.
// It auto-detects .programmator/ in the current working directory for local overrides.
func Load() (*Config, error) {
	globalDir := DefaultConfigDir()

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
	// 1. Start with embedded defaults
	cfg, err := loadEmbedded()
	if err != nil {
		return nil, fmt.Errorf("load embedded defaults: %w", err)
	}
	cfg.sources = append(cfg.sources, "embedded")

	// 2. Merge global config
	globalPath := filepath.Join(globalDir, "config.yaml")
	if overlay, err := loadOverlay(globalPath); err == nil {
		cfg.applyOverlay(overlay)
		cfg.sources = append(cfg.sources, globalPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("load global config: %w", err)
	}

	// 3. Merge local config (highest file precedence)
	if localDir != "" {
		localPath := filepath.Join(localDir, "config.yaml")
		if overlay, err := loadOverlay(localPath); err == nil {
			cfg.applyOverlay(overlay)
			cfg.sources = append(cfg.sources, localPath)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("load local config: %w", err)
		}
	}

	cfg.configDir = globalDir
	cfg.localDir = localDir
	cfg.applyEnvOverrides()

	prompts, err := LoadPrompts(globalDir, localDir)
	if err != nil {
		return nil, fmt.Errorf("load prompts: %w", err)
	}
	cfg.Prompts = prompts

	return cfg, nil
}

// DefaultConfigDir returns the default global configuration directory path.
func DefaultConfigDir() string {
	return dirs.ConfigDir()
}

// loadEmbedded loads config from the embedded defaults.
func loadEmbedded() (*Config, error) {
	data, err := defaultsFS.ReadFile("defaults/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded defaults: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// loadOverlay loads an override config file into a configOverlay.
func loadOverlay(path string) (*configOverlay, error) {
	data, err := os.ReadFile(path) //nolint:gosec // user's config file
	if err != nil {
		return nil, err
	}
	var overlay configOverlay
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &overlay, nil
}

// applyOverlay merges non-nil/non-empty overlay values into the config.
func (c *Config) applyOverlay(o *configOverlay) {
	if o.MaxIterations != nil {
		c.MaxIterations = *o.MaxIterations
	}
	if o.StagnationLimit != nil {
		c.StagnationLimit = *o.StagnationLimit
	}
	if o.Timeout != nil {
		c.Timeout = *o.Timeout
	}
	if o.Executor != "" {
		c.Executor = o.Executor
	}
	if o.Claude.Flags != "" {
		c.Claude.Flags = o.Claude.Flags
	}
	if o.Claude.ConfigDir != "" {
		c.Claude.ConfigDir = o.Claude.ConfigDir
	}
	if o.Claude.AnthropicAPIKey != "" {
		log.Printf("warning: claude.anthropic_api_key loaded from config file — ensure this is a trusted source")
		c.Claude.AnthropicAPIKey = o.Claude.AnthropicAPIKey
	}
	// Pi
	if o.Pi.Flags != "" {
		c.Pi.Flags = o.Pi.Flags
	}
	if o.Pi.ConfigDir != "" {
		c.Pi.ConfigDir = o.Pi.ConfigDir
	}
	if o.Pi.Provider != "" {
		c.Pi.Provider = o.Pi.Provider
	}
	if o.Pi.Model != "" {
		c.Pi.Model = o.Pi.Model
	}
	if o.Pi.APIKey != "" {
		log.Printf("warning: pi.api_key loaded from config file — ensure this is a trusted source")
		c.Pi.APIKey = o.Pi.APIKey
	}
	applyOpenCodeOverlay(&c.OpenCode, &o.OpenCode)
	applyCodexOverlay(&c.Codex, &o.Codex)

	if o.TicketCommand != "" {
		c.TicketCommand = o.TicketCommand
	}

	// Review
	if o.Review.MaxIterations != nil {
		c.Review.MaxIterations = *o.Review.MaxIterations
	}
	if o.Review.Parallel != nil {
		c.Review.Parallel = *o.Review.Parallel
	}
	if o.Review.Executor != nil {
		applyReviewExecutorOverlay(&c.Review.Executor, o.Review.Executor)
	}
	if o.Review.Include != nil {
		c.Review.Include = o.Review.Include
	}
	if o.Review.Exclude != nil {
		c.Review.Exclude = o.Review.Exclude
	}
	if o.Review.Overrides != nil {
		c.Review.Overrides = o.Review.Overrides
	}
	if o.Review.Agents != nil {
		c.Review.Agents = o.Review.Agents
	}
	if o.Review.Validators.Issue != nil {
		c.Review.Validators.Issue = *o.Review.Validators.Issue
	}
	if o.Review.Validators.Simplification != nil {
		c.Review.Validators.Simplification = *o.Review.Validators.Simplification
	}

	// Git
	if o.Git.AutoCommit != nil {
		c.Git.AutoCommit = *o.Git.AutoCommit
	}
	if o.Git.MoveCompletedPlans != nil {
		c.Git.MoveCompletedPlans = *o.Git.MoveCompletedPlans
	}
	if o.Git.CompletedPlansDir != "" {
		c.Git.CompletedPlansDir = o.Git.CompletedPlansDir
	}
	if o.Git.BranchPrefix != "" {
		c.Git.BranchPrefix = o.Git.BranchPrefix
	}
}

func applyReviewExecutorOverlay(dst *ReviewExecutorConfig, src *ReviewExecutorConfig) {
	if src.Name != "" {
		dst.Name = src.Name
	}

	if src.Claude.Flags != "" {
		dst.Claude.Flags = src.Claude.Flags
	}
	if src.Claude.ConfigDir != "" {
		dst.Claude.ConfigDir = src.Claude.ConfigDir
	}
	if src.Claude.AnthropicAPIKey != "" {
		log.Printf("warning: review.executor.claude.anthropic_api_key loaded from config file — ensure this is a trusted source")
		dst.Claude.AnthropicAPIKey = src.Claude.AnthropicAPIKey
	}

	if src.Pi.Flags != "" {
		dst.Pi.Flags = src.Pi.Flags
	}
	if src.Pi.ConfigDir != "" {
		dst.Pi.ConfigDir = src.Pi.ConfigDir
	}
	if src.Pi.Provider != "" {
		dst.Pi.Provider = src.Pi.Provider
	}
	if src.Pi.Model != "" {
		dst.Pi.Model = src.Pi.Model
	}
	if src.Pi.APIKey != "" {
		log.Printf("warning: review.executor.pi.api_key loaded from config file — ensure this is a trusted source")
		dst.Pi.APIKey = src.Pi.APIKey
	}

	if src.OpenCode.Flags != "" {
		dst.OpenCode.Flags = src.OpenCode.Flags
	}
	if src.OpenCode.ConfigDir != "" {
		dst.OpenCode.ConfigDir = src.OpenCode.ConfigDir
	}
	if src.OpenCode.Model != "" {
		dst.OpenCode.Model = src.OpenCode.Model
	}
	if src.OpenCode.APIKey != "" {
		log.Printf("warning: review.executor.opencode.api_key loaded from config file — ensure this is a trusted source")
		dst.OpenCode.APIKey = src.OpenCode.APIKey
	}

	if src.Codex.Flags != "" {
		dst.Codex.Flags = src.Codex.Flags
	}
	if src.Codex.Model != "" {
		dst.Codex.Model = src.Codex.Model
	}
	if src.Codex.APIKey != "" {
		log.Printf("warning: review.executor.codex.api_key loaded from config file — ensure this is a trusted source")
		dst.Codex.APIKey = src.Codex.APIKey
	}
}

func applyCodexOverlay(dst *CodexConfig, src *CodexConfig) {
	if src.Flags != "" {
		dst.Flags = src.Flags
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.APIKey != "" {
		log.Printf("warning: codex.api_key loaded from config file — ensure this is a trusted source")
		dst.APIKey = src.APIKey
	}
}

func applyOpenCodeOverlay(dst *OpenCodeConfig, src *OpenCodeConfig) {
	if src.Flags != "" {
		dst.Flags = src.Flags
	}
	if src.ConfigDir != "" {
		dst.ConfigDir = src.ConfigDir
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.APIKey != "" {
		log.Printf("warning: opencode.api_key loaded from config file — ensure this is a trusted source")
		dst.APIKey = src.APIKey
	}
}

// applyEnvOverrides applies environment variable overrides to the config.
// Env vars override YAML: env > global file > local file > embedded.
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		c.Claude.ConfigDir = v
		c.sources = append(c.sources, "env:CLAUDE_CONFIG_DIR")
	}
}

// ApplyCLIFlags applies CLI flag overrides to the config.
// CLI flags have the highest precedence.
func (c *Config) ApplyCLIFlags(maxIterations, stagnationLimit, timeout int) {
	if maxIterations > 0 {
		c.MaxIterations = maxIterations
		c.sources = append(c.sources, "cli:max-iterations")
	}
	if stagnationLimit > 0 {
		c.StagnationLimit = stagnationLimit
		c.sources = append(c.sources, "cli:stagnation-limit")
	}
	if timeout > 0 {
		c.Timeout = timeout
		c.sources = append(c.sources, "cli:timeout")
	}
}
