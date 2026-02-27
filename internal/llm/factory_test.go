package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInvoker(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ExecutorConfig
		wantType  string
		wantError string
	}{
		{
			name:     "claude executor",
			cfg:      ExecutorConfig{Name: "claude"},
			wantType: "*llm.ClaudeInvoker",
		},
		{
			name:     "empty name defaults to claude",
			cfg:      ExecutorConfig{Name: ""},
			wantType: "*llm.ClaudeInvoker",
		},
		{
			name:     "claude with env config",
			cfg:      ExecutorConfig{Name: "claude", Claude: EnvConfig{AnthropicAPIKey: "test-key"}},
			wantType: "*llm.ClaudeInvoker",
		},
		{
			name:     "pi executor",
			cfg:      ExecutorConfig{Name: "pi"},
			wantType: "*llm.PiInvoker",
		},
		{
			name:      "unknown executor returns error",
			cfg:       ExecutorConfig{Name: "unknown"},
			wantError: `unknown executor: "unknown" (supported: claude, pi)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := NewInvoker(tc.cfg)
			if tc.wantError != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantError, err.Error())
				assert.Nil(t, inv)
			} else {
				require.NoError(t, err)
				require.NotNil(t, inv)
				switch tc.cfg.Name {
				case "pi":
					assert.IsType(t, &PiInvoker{}, inv)
				default:
					assert.IsType(t, &ClaudeInvoker{}, inv)
				}
			}
		})
	}
}

func TestNewInvoker_EnvConfigPassthrough(t *testing.T) {
	envCfg := EnvConfig{
		ClaudeConfigDir: "/custom/config",
		AnthropicAPIKey: "sk-test-key",
	}
	inv, err := NewInvoker(ExecutorConfig{Name: "claude", Claude: envCfg})
	require.NoError(t, err)

	ci, ok := inv.(*ClaudeInvoker)
	require.True(t, ok)
	assert.Equal(t, "/custom/config", ci.Env.ClaudeConfigDir)
	assert.Equal(t, "sk-test-key", ci.Env.AnthropicAPIKey)
}

func TestNewInvoker_PiEnvConfigPassthrough(t *testing.T) {
	piCfg := PiEnvConfig{
		ConfigDir: "/custom/pi/config",
		Provider:  "anthropic",
		Model:     "sonnet",
		APIKey:    "pi-test-key",
	}
	inv, err := NewInvoker(ExecutorConfig{Name: "pi", Pi: piCfg})
	require.NoError(t, err)

	pi, ok := inv.(*PiInvoker)
	require.True(t, ok)
	assert.Equal(t, "/custom/pi/config", pi.Env.ConfigDir)
	assert.Equal(t, "anthropic", pi.Env.Provider)
	assert.Equal(t, "sonnet", pi.Env.Model)
	assert.Equal(t, "pi-test-key", pi.Env.APIKey)
}
