package executor

import (
	"testing"

	"github.com/alexander-akhmetov/programmator/internal/llm/claude"
	"github.com/alexander-akhmetov/programmator/internal/llm/opencode"
	"github.com/alexander-akhmetov/programmator/internal/llm/pi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantType  any
		wantError string
	}{
		{
			name:     "claude executor",
			cfg:      Config{Name: "claude"},
			wantType: &claude.Invoker{},
		},
		{
			name:     "empty name defaults to claude",
			cfg:      Config{Name: ""},
			wantType: &claude.Invoker{},
		},
		{
			name:     "claude with env config",
			cfg:      Config{Name: "claude", Claude: claude.Config{AnthropicAPIKey: "test-key"}},
			wantType: &claude.Invoker{},
		},
		{
			name:     "pi executor",
			cfg:      Config{Name: "pi"},
			wantType: &pi.Invoker{},
		},
		{
			name:     "opencode executor",
			cfg:      Config{Name: "opencode"},
			wantType: &opencode.Invoker{},
		},
		{
			name:      "unknown executor returns error",
			cfg:       Config{Name: "unknown"},
			wantError: `unknown executor: "unknown" (supported: claude, pi, opencode)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := New(tc.cfg)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantError, err.Error())
				assert.Nil(t, inv)
			} else {
				require.NoError(t, err)
				require.NotNil(t, inv)
				assert.IsType(t, tc.wantType, inv)
			}
		})
	}
}

func TestNew_ClaudeConfigPassthrough(t *testing.T) {
	envCfg := claude.Config{
		ClaudeConfigDir: "/custom/config",
		AnthropicAPIKey: "sk-test-key",
	}
	inv, err := New(Config{Name: "claude", Claude: envCfg})
	require.NoError(t, err)

	ci, ok := inv.(*claude.Invoker)
	require.True(t, ok)
	assert.Equal(t, "/custom/config", ci.Env.ClaudeConfigDir)
	assert.Equal(t, "sk-test-key", ci.Env.AnthropicAPIKey)
}

func TestNew_PiConfigPassthrough(t *testing.T) {
	piCfg := pi.Config{
		ConfigDir: "/custom/pi/config",
		Provider:  "anthropic",
		Model:     "sonnet",
		APIKey:    "pi-test-key",
	}
	inv, err := New(Config{Name: "pi", Pi: piCfg})
	require.NoError(t, err)

	p, ok := inv.(*pi.Invoker)
	require.True(t, ok)
	assert.Equal(t, "/custom/pi/config", p.Env.ConfigDir)
	assert.Equal(t, "anthropic", p.Env.Provider)
	assert.Equal(t, "sonnet", p.Env.Model)
	assert.Equal(t, "pi-test-key", p.Env.APIKey)
}

func TestNew_OpenCodeConfigPassthrough(t *testing.T) {
	ocCfg := opencode.Config{
		Model:     "anthropic/claude-sonnet-4-5",
		APIKey:    "oc-test-key",
		ConfigDir: "/custom/opencode/config",
	}
	inv, err := New(Config{Name: "opencode", OpenCode: ocCfg})
	require.NoError(t, err)

	oc, ok := inv.(*opencode.Invoker)
	require.True(t, ok)
	assert.Equal(t, "anthropic/claude-sonnet-4-5", oc.Env.Model)
	assert.Equal(t, "oc-test-key", oc.Env.APIKey)
	assert.Equal(t, "/custom/opencode/config", oc.Env.ConfigDir)
}
