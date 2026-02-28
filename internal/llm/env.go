package llm

import (
	"strings"
)

// ProviderAPIKeyEnvVars maps provider names to their expected API key env var.
// Shared by pi and opencode executors.
var ProviderAPIKeyEnvVars = map[string]string{
	"anthropic": "ANTHROPIC_API_KEY",
	"openai":    "OPENAI_API_KEY",
	"google":    "GEMINI_API_KEY",
	"groq":      "GROQ_API_KEY",
	"mistral":   "MISTRAL_API_KEY",
}

// AllProviderAPIKeyPrefixes returns all known provider API key env var prefixes
// (with trailing "=") for use in env filtering.
func AllProviderAPIKeyPrefixes() []string {
	prefixes := make([]string, 0, len(ProviderAPIKeyEnvVars))
	for _, v := range ProviderAPIKeyEnvVars {
		prefixes = append(prefixes, v+"=")
	}
	return prefixes
}

// FilterEnv returns a copy of environ with entries matching any of the given
// prefixes removed. Each prefix should include a trailing "=" to match env
// var assignments (e.g. "ANTHROPIC_API_KEY=").
func FilterEnv(environ []string, excludePrefixes ...string) []string {
	result := make([]string, 0, len(environ))
	for _, e := range environ {
		filtered := false
		for _, prefix := range excludePrefixes {
			if strings.HasPrefix(e, prefix) {
				filtered = true
				break
			}
		}
		if !filtered {
			result = append(result, e)
		}
	}
	return result
}
