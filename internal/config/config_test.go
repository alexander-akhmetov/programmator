package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEmbedded(t *testing.T) {
	cfg, err := loadEmbedded()
	require.NoError(t, err)

	// Check defaults from embedded config
	assert.Equal(t, 50, cfg.MaxIterations)
	assert.Equal(t, 3, cfg.StagnationLimit)
	assert.Equal(t, 900, cfg.Timeout)
	assert.Equal(t, "", cfg.ClaudeFlags)
	assert.Equal(t, false, cfg.Review.Enabled)
	assert.Equal(t, 3, cfg.Review.MaxIterations)
	assert.Len(t, cfg.Review.Passes, 2)
}

func TestInstallDefaults(t *testing.T) {
	tmpDir := t.TempDir()

	err := InstallDefaults(tmpDir)
	require.NoError(t, err)

	// Check config file was created
	configPath := filepath.Join(tmpDir, "config.yaml")
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Read and verify content
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "max_iterations: 50")
}

func TestInstallDefaultsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	// First install
	err := InstallDefaults(tmpDir)
	require.NoError(t, err)

	// Modify the config
	configPath := filepath.Join(tmpDir, "config.yaml")
	err = os.WriteFile(configPath, []byte("max_iterations: 100\n"), 0o600)
	require.NoError(t, err)

	// Second install should not overwrite
	err = InstallDefaults(tmpDir)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "max_iterations: 100")
}

func TestLoadWithDirs_GlobalOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create global config
	err := os.WriteFile(
		filepath.Join(tmpDir, "config.yaml"),
		[]byte("max_iterations: 100\nstagnation_limit: 5\n"),
		0o600,
	)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(tmpDir, "")
	require.NoError(t, err)

	assert.Equal(t, 100, cfg.MaxIterations)
	assert.Equal(t, 5, cfg.StagnationLimit)
	assert.Equal(t, 900, cfg.Timeout) // from embedded default
}

func TestLoadWithDirs_LocalOverridesGlobal(t *testing.T) {
	globalDir := t.TempDir()
	localDir := t.TempDir()

	// Create global config
	err := os.WriteFile(
		filepath.Join(globalDir, "config.yaml"),
		[]byte("max_iterations: 100\nstagnation_limit: 5\n"),
		0o600,
	)
	require.NoError(t, err)

	// Create local config that overrides max_iterations
	err = os.WriteFile(
		filepath.Join(localDir, "config.yaml"),
		[]byte("max_iterations: 25\n"),
		0o600,
	)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(globalDir, localDir)
	require.NoError(t, err)

	assert.Equal(t, 25, cfg.MaxIterations)  // from local
	assert.Equal(t, 5, cfg.StagnationLimit) // from global
	assert.Equal(t, 900, cfg.Timeout)       // from embedded default
}

func TestApplyEnv(t *testing.T) {
	// Save and restore env vars
	oldMaxIter := os.Getenv("PROGRAMMATOR_MAX_ITERATIONS")
	oldStag := os.Getenv("PROGRAMMATOR_STAGNATION_LIMIT")
	defer func() {
		os.Setenv("PROGRAMMATOR_MAX_ITERATIONS", oldMaxIter)
		os.Setenv("PROGRAMMATOR_STAGNATION_LIMIT", oldStag)
	}()

	os.Setenv("PROGRAMMATOR_MAX_ITERATIONS", "75")
	os.Setenv("PROGRAMMATOR_STAGNATION_LIMIT", "10")

	cfg, err := loadEmbedded()
	require.NoError(t, err)

	cfg.applyEnv()

	assert.Equal(t, 75, cfg.MaxIterations)
	assert.Equal(t, 10, cfg.StagnationLimit)
	assert.True(t, cfg.MaxIterationsSet)
	assert.True(t, cfg.StagnationLimitSet)
}

func TestEnvBetweenGlobalAndLocal(t *testing.T) {
	// Env vars should be between global and local in precedence
	// Order: embedded → global → env → local

	globalDir := t.TempDir()
	localDir := t.TempDir()

	// Global sets max_iterations to 100
	err := os.WriteFile(
		filepath.Join(globalDir, "config.yaml"),
		[]byte("max_iterations: 100\n"),
		0o600,
	)
	require.NoError(t, err)

	// Env sets stagnation_limit to 7
	oldStag := os.Getenv("PROGRAMMATOR_STAGNATION_LIMIT")
	defer os.Setenv("PROGRAMMATOR_STAGNATION_LIMIT", oldStag)
	os.Setenv("PROGRAMMATOR_STAGNATION_LIMIT", "7")

	// Local sets timeout to 600
	err = os.WriteFile(
		filepath.Join(localDir, "config.yaml"),
		[]byte("timeout: 600\n"),
		0o600,
	)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(globalDir, localDir)
	require.NoError(t, err)

	assert.Equal(t, 100, cfg.MaxIterations) // from global
	assert.Equal(t, 7, cfg.StagnationLimit) // from env
	assert.Equal(t, 600, cfg.Timeout)       // from local
}

func TestLocalOverridesEnv(t *testing.T) {
	// Local config should override env vars
	globalDir := t.TempDir()
	localDir := t.TempDir()

	// Env sets max_iterations
	oldMaxIter := os.Getenv("PROGRAMMATOR_MAX_ITERATIONS")
	defer os.Setenv("PROGRAMMATOR_MAX_ITERATIONS", oldMaxIter)
	os.Setenv("PROGRAMMATOR_MAX_ITERATIONS", "75")

	// Local also sets max_iterations (should win)
	err := os.WriteFile(
		filepath.Join(localDir, "config.yaml"),
		[]byte("max_iterations: 30\n"),
		0o600,
	)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(globalDir, localDir)
	require.NoError(t, err)

	assert.Equal(t, 30, cfg.MaxIterations) // local wins over env
}

func TestApplyCLIFlags(t *testing.T) {
	cfg, err := loadEmbedded()
	require.NoError(t, err)

	cfg.ApplyCLIFlags(200, 15, 1800)

	assert.Equal(t, 200, cfg.MaxIterations)
	assert.Equal(t, 15, cfg.StagnationLimit)
	assert.Equal(t, 1800, cfg.Timeout)
}

func TestApplyCLIFlagsZeroNoOverride(t *testing.T) {
	cfg, err := loadEmbedded()
	require.NoError(t, err)

	// Zero values should not override
	cfg.ApplyCLIFlags(0, 0, 0)

	assert.Equal(t, 50, cfg.MaxIterations)  // unchanged
	assert.Equal(t, 3, cfg.StagnationLimit) // unchanged
	assert.Equal(t, 900, cfg.Timeout)       // unchanged
}

func TestReviewConfig(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `
review:
  enabled: true
  max_iterations: 5
  passes:
    - name: custom_review
      parallel: true
      agents:
        - name: custom_agent
          focus:
            - custom focus
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0o600)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(tmpDir, "")
	require.NoError(t, err)

	assert.True(t, cfg.Review.Enabled)
	assert.Equal(t, 5, cfg.Review.MaxIterations)
	require.Len(t, cfg.Review.Passes, 1)
	assert.Equal(t, "custom_review", cfg.Review.Passes[0].Name)
	require.Len(t, cfg.Review.Passes[0].Agents, 1)
	assert.Equal(t, "custom_agent", cfg.Review.Passes[0].Agents[0].Name)
}

func TestDefaultConfigDir(t *testing.T) {
	dir := DefaultConfigDir()
	assert.Contains(t, dir, "programmator")
	assert.Contains(t, dir, ".config")
}

func TestSources(t *testing.T) {
	globalDir := t.TempDir()
	localDir := t.TempDir()

	err := os.WriteFile(
		filepath.Join(globalDir, "config.yaml"),
		[]byte("max_iterations: 100\n"),
		0o600,
	)
	require.NoError(t, err)

	err = os.WriteFile(
		filepath.Join(localDir, "config.yaml"),
		[]byte("stagnation_limit: 5\n"),
		0o600,
	)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(globalDir, localDir)
	require.NoError(t, err)

	sources := cfg.Sources()
	assert.Contains(t, sources, "embedded")
	assert.Contains(t, sources, filepath.Join(globalDir, "config.yaml"))
	assert.Contains(t, sources, filepath.Join(localDir, "config.yaml"))
}

func TestParseConfigWithTracking(t *testing.T) {
	data := []byte(`
max_iterations: 100
review:
  enabled: true
`)
	cfg, err := parseConfigWithTracking(data)
	require.NoError(t, err)

	assert.True(t, cfg.MaxIterationsSet)
	assert.False(t, cfg.StagnationLimitSet) // not set in YAML
	assert.False(t, cfg.TimeoutSet)         // not set in YAML
	assert.True(t, cfg.Review.EnabledSet)
}
