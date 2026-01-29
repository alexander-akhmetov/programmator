// Package config provides unified configuration management for programmator.
// Configuration is loaded from multiple sources with the following precedence:
// embedded defaults → global file → env vars → local file → CLI flags
package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

//go:embed defaults/config.yaml
var defaultsFS embed.FS

// ReviewAgentConfig defines a single review agent configuration.
type ReviewAgentConfig struct {
	Name   string   `yaml:"name"`
	Focus  []string `yaml:"focus"`
	Prompt string   `yaml:"prompt,omitempty"`
}

// ReviewPass defines a review pass with multiple agents.
type ReviewPass struct {
	Name     string              `yaml:"name"`
	Parallel bool                `yaml:"parallel"`
	Agents   []ReviewAgentConfig `yaml:"agents"`
}

// ReviewConfig holds review-specific configuration.
type ReviewConfig struct {
	Enabled       bool         `yaml:"enabled"`
	MaxIterations int          `yaml:"max_iterations"`
	Passes        []ReviewPass `yaml:"passes"`

	// Set tracking for merge
	EnabledSet       bool `yaml:"-"`
	MaxIterationsSet bool `yaml:"-"`
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

	// Progress log settings
	LogsDir string `yaml:"logs_dir"` // Directory for progress logs (default: ~/.programmator/logs)

	// Git workflow settings
	Git GitConfig `yaml:"git"`

	// Review settings (nested)
	Review ReviewConfig `yaml:"review"`

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
	// Install defaults to global dir if not exists
	if err := InstallDefaults(globalDir); err != nil {
		return nil, fmt.Errorf("install defaults: %w", err)
	}

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

// InstallDefaults creates the config directory and installs default config if not exists.
func InstallDefaults(configDir string) error {
	// Create config directory
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Create prompts directory (for user overrides)
	promptsDir := filepath.Join(configDir, "prompts")
	if err := os.MkdirAll(promptsDir, 0o700); err != nil {
		return fmt.Errorf("create prompts dir: %w", err)
	}

	// Install default config file if not exists
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		data, err := defaultsFS.ReadFile("defaults/config.yaml")
		if err != nil {
			return fmt.Errorf("read embedded config: %w", err)
		}
		if err := os.WriteFile(configPath, data, 0o600); err != nil {
			return fmt.Errorf("write config file: %w", err)
		}
	}

	return nil
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
		if _, ok := review["enabled"]; ok {
			cfg.Review.EnabledSet = true
		}
		if _, ok := review["max_iterations"]; ok {
			cfg.Review.MaxIterationsSet = true
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
		}
	}

	if v := os.Getenv("PROGRAMMATOR_STAGNATION_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.StagnationLimit = n
			c.StagnationLimitSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_STAGNATION_LIMIT")
		}
	}

	if v := os.Getenv("PROGRAMMATOR_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Timeout = n
			c.TimeoutSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_TIMEOUT")
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

	if v := os.Getenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Review.MaxIterations = n
			c.Review.MaxIterationsSet = true
			c.sources = append(c.sources, "env:PROGRAMMATOR_MAX_REVIEW_ITERATIONS")
		}
	}

	if v := os.Getenv("PROGRAMMATOR_REVIEW_ENABLED"); v != "" {
		c.Review.Enabled = v == "true" || v == "1"
		c.Review.EnabledSet = true
		c.sources = append(c.sources, "env:PROGRAMMATOR_REVIEW_ENABLED")
	}
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
	if src.LogsDir != "" {
		c.LogsDir = src.LogsDir
	}

	// Review config merge
	if src.Review.EnabledSet {
		c.Review.Enabled = src.Review.Enabled
		c.Review.EnabledSet = true
	}
	if src.Review.MaxIterationsSet {
		c.Review.MaxIterations = src.Review.MaxIterations
		c.Review.MaxIterationsSet = true
	}
	if len(src.Review.Passes) > 0 {
		c.Review.Passes = src.Review.Passes
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
