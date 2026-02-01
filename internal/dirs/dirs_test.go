package dirs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "default uses ~/.config/programmator",
			envVars:  map[string]string{"XDG_CONFIG_HOME": ""},
			expected: filepath.Join(home, ".config", "programmator"),
		},
		{
			name:     "respects XDG_CONFIG_HOME",
			envVars:  map[string]string{"XDG_CONFIG_HOME": "/custom/config"},
			expected: filepath.Join("/custom/config", "programmator"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.expected, ConfigDir())
		})
	}
}

func TestStateDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "default uses ~/.local/state/programmator",
			envVars:  map[string]string{"XDG_STATE_HOME": "", "PROGRAMMATOR_STATE_DIR": ""},
			expected: filepath.Join(home, ".local", "state", "programmator"),
		},
		{
			name:     "respects XDG_STATE_HOME",
			envVars:  map[string]string{"XDG_STATE_HOME": "/custom/state", "PROGRAMMATOR_STATE_DIR": ""},
			expected: filepath.Join("/custom/state", "programmator"),
		},
		{
			name:     "PROGRAMMATOR_STATE_DIR takes precedence over XDG_STATE_HOME",
			envVars:  map[string]string{"PROGRAMMATOR_STATE_DIR": "/override/dir", "XDG_STATE_HOME": "/custom/state"},
			expected: "/override/dir",
		},
		{
			name:     "PROGRAMMATOR_STATE_DIR alone",
			envVars:  map[string]string{"PROGRAMMATOR_STATE_DIR": "/override/dir", "XDG_STATE_HOME": ""},
			expected: "/override/dir",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.expected, StateDir())
		})
	}
}

func TestLogsDir(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "default is StateDir/logs",
			envVars:  map[string]string{"XDG_STATE_HOME": "", "PROGRAMMATOR_STATE_DIR": ""},
			expected: filepath.Join(home, ".local", "state", "programmator", "logs"),
		},
		{
			name:     "follows XDG_STATE_HOME",
			envVars:  map[string]string{"XDG_STATE_HOME": "/custom/state", "PROGRAMMATOR_STATE_DIR": ""},
			expected: filepath.Join("/custom/state", "programmator", "logs"),
		},
		{
			name:     "follows PROGRAMMATOR_STATE_DIR",
			envVars:  map[string]string{"PROGRAMMATOR_STATE_DIR": "/override", "XDG_STATE_HOME": ""},
			expected: filepath.Join("/override", "logs"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.envVars {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.expected, LogsDir())
		})
	}
}
