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
	require.Len(t, cfg.Phases, 3)

	// Phase 1: comprehensive
	require.Equal(t, "comprehensive", cfg.Phases[0].Name)
	require.Equal(t, 1, cfg.Phases[0].IterationLimit)
	require.True(t, cfg.Phases[0].Parallel)
	require.Len(t, cfg.Phases[0].Agents, 6)

	// Phase 2: critical_loop
	require.Equal(t, "critical_loop", cfg.Phases[1].Name)
	require.Equal(t, 10, cfg.Phases[1].IterationPct)
	require.Len(t, cfg.Phases[1].SeverityFilter, 2)

	// Phase 3: final_check
	require.Equal(t, "final_check", cfg.Phases[2].Name)
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
	t.Run("valid config file with phases", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "review.yaml")

		content := `enabled: true
max_iterations: 5
phases:
  - name: custom_phase
    iteration_limit: 2
    parallel: true
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
		require.Len(t, cfg.Phases, 1)
		require.Equal(t, "custom_phase", cfg.Phases[0].Name)
		require.True(t, cfg.Phases[0].Parallel)
		require.Equal(t, 2, cfg.Phases[0].IterationLimit)
		require.Len(t, cfg.Phases[0].Agents, 1)
		require.Equal(t, "custom_agent", cfg.Phases[0].Agents[0].Name)
		require.Equal(t, []string{"focus1", "focus2"}, cfg.Phases[0].Agents[0].Focus)
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
		Phases:        []Phase{{Name: "env_phase"}},
	}

	t.Run("file takes precedence", func(t *testing.T) {
		file := Config{
			Enabled:       true,
			MaxIterations: 5,
			Phases:        []Phase{{Name: "file_phase"}},
		}

		result := mergeConfigs(env, file)
		require.True(t, result.Enabled)
		require.Equal(t, 5, result.MaxIterations)
		require.Equal(t, "file_phase", result.Phases[0].Name)
	})

	t.Run("env used when file empty", func(t *testing.T) {
		file := Config{
			Enabled:       true,
			MaxIterations: 0,
			Phases:        nil,
		}

		result := mergeConfigs(env, file)
		require.True(t, result.Enabled)
		require.Equal(t, 3, result.MaxIterations)
		require.Equal(t, "env_phase", result.Phases[0].Name)
	})
}

func TestPhaseMaxIterations(t *testing.T) {
	t.Run("static limit takes precedence", func(t *testing.T) {
		phase := Phase{
			Name:           "test",
			IterationLimit: 5,
			IterationPct:   20, // Should be ignored
		}
		require.Equal(t, 5, phase.MaxIterations(50))
	})

	t.Run("percentage calculation", func(t *testing.T) {
		phase := Phase{
			Name:         "test",
			IterationPct: 10,
		}
		// 10% of 50 = 5
		require.Equal(t, 5, phase.MaxIterations(50))
	})

	t.Run("percentage minimum is 3", func(t *testing.T) {
		phase := Phase{
			Name:         "test",
			IterationPct: 1, // 1% of 50 = 0.5, rounds to 0
		}
		// Should return minimum of 3
		require.Equal(t, 3, phase.MaxIterations(50))
	})

	t.Run("default when nothing set", func(t *testing.T) {
		phase := Phase{
			Name: "test",
		}
		require.Equal(t, 3, phase.MaxIterations(50))
	})
}

func TestDefaultConfigHasPhases(t *testing.T) {
	cfg := DefaultConfig()

	require.NotEmpty(t, cfg.Phases, "default config should have phases")
	require.Len(t, cfg.Phases, 3, "default config should have 3 phases")

	// Phase 1: comprehensive
	require.Equal(t, "comprehensive", cfg.Phases[0].Name)
	require.Equal(t, 1, cfg.Phases[0].IterationLimit)
	require.True(t, cfg.Phases[0].Parallel)
	require.Len(t, cfg.Phases[0].Agents, 6) // quality, security, implementation, testing, simplification, linter
	require.Empty(t, cfg.Phases[0].SeverityFilter)

	// Phase 2: critical_loop
	require.Equal(t, "critical_loop", cfg.Phases[1].Name)
	require.Equal(t, 10, cfg.Phases[1].IterationPct)
	require.Len(t, cfg.Phases[1].SeverityFilter, 2)
	require.Contains(t, cfg.Phases[1].SeverityFilter, SeverityCritical)
	require.Contains(t, cfg.Phases[1].SeverityFilter, SeverityHigh)

	// Phase 3: final_check
	require.Equal(t, "final_check", cfg.Phases[2].Name)
	require.Equal(t, 10, cfg.Phases[2].IterationPct)
}
