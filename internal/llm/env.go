package llm

import (
	"os"
	"strings"
)

// EnvConfig holds environment configuration for Claude subprocesses.
type EnvConfig struct {
	ClaudeConfigDir string
	AnthropicAPIKey string
}

// BuildEnv constructs the environment variable slice for a Claude subprocess.
// It filters ANTHROPIC_API_KEY from the inherited environment and only sets
// it if explicitly configured via the EnvConfig.
func BuildEnv(cfg EnvConfig) []string {
	environ := os.Environ()
	env := make([]string, 0, len(environ))
	for _, e := range environ {
		if !strings.HasPrefix(e, "ANTHROPIC_API_KEY=") && !strings.HasPrefix(e, "CLAUDE_CONFIG_DIR=") {
			env = append(env, e)
		}
	}
	if cfg.ClaudeConfigDir != "" {
		env = append(env, "CLAUDE_CONFIG_DIR="+cfg.ClaudeConfigDir)
	}
	if cfg.AnthropicAPIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+cfg.AnthropicAPIKey)
	}
	return env
}
