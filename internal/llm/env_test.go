package llm

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildEnv(t *testing.T) {
	tests := []struct {
		name       string
		setEnv     map[string]string
		config     EnvConfig
		wantSet    map[string]string // key=value pairs that must be present
		wantAbsent []string          // prefixes that must NOT be present
	}{
		{
			name:       "filters inherited ANTHROPIC_API_KEY",
			setEnv:     map[string]string{"ANTHROPIC_API_KEY": "secret-inherited"},
			config:     EnvConfig{},
			wantAbsent: []string{"ANTHROPIC_API_KEY="},
		},
		{
			name:    "sets explicit ANTHROPIC_API_KEY",
			setEnv:  map[string]string{"ANTHROPIC_API_KEY": "should-be-filtered"},
			config:  EnvConfig{AnthropicAPIKey: "explicit-key"},
			wantSet: map[string]string{"ANTHROPIC_API_KEY": "explicit-key"},
		},
		{
			name:    "sets CLAUDE_CONFIG_DIR",
			config:  EnvConfig{ClaudeConfigDir: "/custom/config"},
			wantSet: map[string]string{"CLAUDE_CONFIG_DIR": "/custom/config"},
		},
		{
			name:   "empty config returns non-nil env",
			config: EnvConfig{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.setEnv {
				t.Setenv(k, v)
			}

			env := BuildEnv(tc.config)
			require.NotNil(t, env)

			for key, val := range tc.wantSet {
				expected := key + "=" + val
				require.True(t, slices.Contains(env, expected),
					"%s should be set in env", expected)
			}

			for _, prefix := range tc.wantAbsent {
				for _, e := range env {
					require.False(t, strings.HasPrefix(e, prefix),
						"%s should not be in env", prefix)
				}
			}
		})
	}
}
