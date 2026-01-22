package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	require.False(t, cfg.Enabled)
	require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	require.Len(t, cfg.Passes, 2)

	// First pass: code_review
	require.Equal(t, "code_review", cfg.Passes[0].Name)
	require.True(t, cfg.Passes[0].Parallel)
	require.Len(t, cfg.Passes[0].Agents, 2)
	require.Equal(t, "quality", cfg.Passes[0].Agents[0].Name)
	require.Equal(t, "security", cfg.Passes[0].Agents[1].Name)

	// Second pass: linter
	require.Equal(t, "linter", cfg.Passes[1].Name)
	require.False(t, cfg.Passes[1].Parallel)
	require.Len(t, cfg.Passes[1].Agents, 1)
	require.Equal(t, "linter", cfg.Passes[1].Agents[0].Name)
}

func TestConfigFromEnv(t *testing.T) {
	// Clean env before test
	os.Unsetenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS")
	os.Unsetenv("PROGRAMMATOR_REVIEW_ENABLED")
	os.Unsetenv("PROGRAMMATOR_REVIEW_CONFIG")

	t.Run("default values", func(t *testing.T) {
		cfg := ConfigFromEnv()
		require.False(t, cfg.Enabled)
		require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	})

	t.Run("env override max iterations", func(t *testing.T) {
		os.Setenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS", "5")
		defer os.Unsetenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS")

		cfg := ConfigFromEnv()
		require.Equal(t, 5, cfg.MaxIterations)
	})

	t.Run("env override enabled true", func(t *testing.T) {
		os.Setenv("PROGRAMMATOR_REVIEW_ENABLED", "true")
		defer os.Unsetenv("PROGRAMMATOR_REVIEW_ENABLED")

		cfg := ConfigFromEnv()
		require.True(t, cfg.Enabled)
	})

	t.Run("env override enabled 1", func(t *testing.T) {
		os.Setenv("PROGRAMMATOR_REVIEW_ENABLED", "1")
		defer os.Unsetenv("PROGRAMMATOR_REVIEW_ENABLED")

		cfg := ConfigFromEnv()
		require.True(t, cfg.Enabled)
	})

	t.Run("invalid max iterations ignored", func(t *testing.T) {
		os.Setenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS", "invalid")
		defer os.Unsetenv("PROGRAMMATOR_MAX_REVIEW_ITERATIONS")

		cfg := ConfigFromEnv()
		require.Equal(t, DefaultMaxIterations, cfg.MaxIterations)
	})
}

func TestLoadConfigFile(t *testing.T) {
	t.Run("valid config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "review.yaml")

		content := `enabled: true
max_iterations: 5
passes:
  - name: custom_pass
    parallel: false
    agents:
      - name: custom_agent
        focus:
          - focus1
          - focus2
`
		err := os.WriteFile(configPath, []byte(content), 0644)
		require.NoError(t, err)

		cfg, err := LoadConfigFile(configPath)
		require.NoError(t, err)
		require.True(t, cfg.Enabled)
		require.Equal(t, 5, cfg.MaxIterations)
		require.Len(t, cfg.Passes, 1)
		require.Equal(t, "custom_pass", cfg.Passes[0].Name)
		require.False(t, cfg.Passes[0].Parallel)
		require.Len(t, cfg.Passes[0].Agents, 1)
		require.Equal(t, "custom_agent", cfg.Passes[0].Agents[0].Name)
		require.Equal(t, []string{"focus1", "focus2"}, cfg.Passes[0].Agents[0].Focus)
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadConfigFile("/nonexistent/path/config.yaml")
		require.Error(t, err)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "review.yaml")

		content := `invalid: yaml: content:`
		err := os.WriteFile(configPath, []byte(content), 0644)
		require.NoError(t, err)

		_, err = LoadConfigFile(configPath)
		require.Error(t, err)
	})
}

func TestMergeConfigs(t *testing.T) {
	env := Config{
		Enabled:       false,
		MaxIterations: 3,
		Passes:        []Pass{{Name: "env_pass"}},
	}

	t.Run("file takes precedence", func(t *testing.T) {
		file := Config{
			Enabled:       true,
			MaxIterations: 5,
			Passes:        []Pass{{Name: "file_pass"}},
		}

		result := mergeConfigs(env, file)
		require.True(t, result.Enabled)
		require.Equal(t, 5, result.MaxIterations)
		require.Equal(t, "file_pass", result.Passes[0].Name)
	})

	t.Run("env used when file empty", func(t *testing.T) {
		file := Config{
			Enabled:       true,
			MaxIterations: 0,
			Passes:        nil,
		}

		result := mergeConfigs(env, file)
		require.True(t, result.Enabled)
		require.Equal(t, 3, result.MaxIterations)
		require.Equal(t, "env_pass", result.Passes[0].Name)
	})
}
