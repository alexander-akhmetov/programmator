package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildHookSettings(t *testing.T) {
	t.Setenv("HOME", "/mock/home")
	dcgExpect := "DCG_CONFIG='/mock/home/.config/dcg/config.toml' dcg"

	tests := []struct {
		name           string
		config         HookConfig
		wantContains   []string
		wantNotContain []string
		wantEmpty      bool
	}{
		{
			name:   "permission only",
			config: HookConfig{PermissionSocketPath: "/tmp/test.sock"},
			wantContains: []string{
				`"matcher":""`,
				"programmator hook --socket /tmp/test.sock",
				`"timeout":120000`,
			},
			wantNotContain: []string{"dcg"},
		},
		{
			name:   "guard only",
			config: HookConfig{GuardMode: true},
			wantContains: []string{
				`"matcher":"Bash"`,
				dcgExpect,
				`"timeout":5000`,
			},
			wantNotContain: []string{"programmator hook"},
		},
		{
			name:   "both combined",
			config: HookConfig{PermissionSocketPath: "/tmp/test.sock", GuardMode: true},
			wantContains: []string{
				`"matcher":""`,
				"programmator hook --socket /tmp/test.sock",
				`"matcher":"Bash"`,
				dcgExpect,
			},
		},
		{
			name:      "empty config",
			config:    HookConfig{},
			wantEmpty: true,
		},
		{
			name:      "unsafe path rejected",
			config:    HookConfig{PermissionSocketPath: "/tmp/test;rm -rf /"},
			wantEmpty: true,
		},
		{
			name:      "path with spaces rejected",
			config:    HookConfig{PermissionSocketPath: "/tmp/my socket"},
			wantEmpty: true,
		},
		{
			name:      "backtick injection rejected",
			config:    HookConfig{PermissionSocketPath: "/tmp/`rm -rf /`"},
			wantEmpty: true,
		},
		{
			name:      "command substitution rejected",
			config:    HookConfig{PermissionSocketPath: "/tmp/$(id)"},
			wantEmpty: true,
		},
		{
			name:      "quote injection rejected",
			config:    HookConfig{PermissionSocketPath: "/tmp/test'path"},
			wantEmpty: true,
		},
		{
			name:   "unsafe socket path still allows guard mode",
			config: HookConfig{PermissionSocketPath: "/tmp/bad;path", GuardMode: true},
			wantContains: []string{
				dcgExpect,
				`"matcher":"Bash"`,
			},
			wantNotContain: []string{"programmator hook"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := BuildHookSettings(tc.config)

			if tc.wantEmpty {
				require.Equal(t, "", settings)
				return
			}

			for _, s := range tc.wantContains {
				require.Contains(t, settings, s)
			}
			for _, s := range tc.wantNotContain {
				require.NotContains(t, settings, s)
			}
		})
	}
}
