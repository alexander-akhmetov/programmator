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

	assert.Equal(t, 50, cfg.MaxIterations)
	assert.Equal(t, 3, cfg.StagnationLimit)
	assert.Equal(t, 900, cfg.Timeout)
	assert.Equal(t, "claude", cfg.Executor)
	assert.Equal(t, "", cfg.Claude.Flags)
	assert.Equal(t, 3, cfg.Review.MaxIterations)
	assert.True(t, cfg.Review.Parallel)
	assert.Len(t, cfg.Review.Agents, 8)
}

func TestLoadWithDirs_GlobalOnly(t *testing.T) {
	tmpDir := t.TempDir()

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

	err := os.WriteFile(
		filepath.Join(globalDir, "config.yaml"),
		[]byte("max_iterations: 100\nstagnation_limit: 5\n"),
		0o600,
	)
	require.NoError(t, err)

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

func TestLoadWithDirs_LocalOverridesWithZero(t *testing.T) {
	globalDir := t.TempDir()
	localDir := t.TempDir()

	err := os.WriteFile(
		filepath.Join(globalDir, "config.yaml"),
		[]byte("max_iterations: 100\nstagnation_limit: 5\n"),
		0o600,
	)
	require.NoError(t, err)

	// Local explicitly sets stagnation_limit to 0
	err = os.WriteFile(
		filepath.Join(localDir, "config.yaml"),
		[]byte("stagnation_limit: 0\n"),
		0o600,
	)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(globalDir, localDir)
	require.NoError(t, err)

	assert.Equal(t, 100, cfg.MaxIterations) // from global
	assert.Equal(t, 0, cfg.StagnationLimit) // local overrides to 0
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

	cfg.ApplyCLIFlags(0, 0, 0)

	assert.Equal(t, 50, cfg.MaxIterations)  // unchanged
	assert.Equal(t, 3, cfg.StagnationLimit) // unchanged
	assert.Equal(t, 900, cfg.Timeout)       // unchanged
}

func TestReviewAgentsConfig(t *testing.T) {
	tmpDir := t.TempDir()

	configContent := `
review:
  max_iterations: 3
  agents:
    - name: quality
      focus:
        - error handling
        - concurrency
      prompt: custom_prompt.md
    - name: security
      focus:
        - injection
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0o600)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(tmpDir, "")
	require.NoError(t, err)

	assert.Equal(t, 3, cfg.Review.MaxIterations)
	require.Len(t, cfg.Review.Agents, 2)
	assert.Equal(t, "quality", cfg.Review.Agents[0].Name)
	assert.Equal(t, []string{"error handling", "concurrency"}, cfg.Review.Agents[0].Focus)
	assert.Equal(t, "custom_prompt.md", cfg.Review.Agents[0].Prompt)
	assert.Equal(t, "security", cfg.Review.Agents[1].Name)
	assert.Equal(t, []string{"injection"}, cfg.Review.Agents[1].Focus)
	assert.Empty(t, cfg.Review.Agents[1].Prompt)
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

func TestApplyOverlay_Executor(t *testing.T) {
	tests := []struct {
		name         string
		baseExec     string
		overrideExec string
		wantExec     string
	}{
		{
			name:         "override replaces base",
			baseExec:     "claude",
			overrideExec: "custom",
			wantExec:     "custom",
		},
		{
			name:         "empty override keeps base",
			baseExec:     "claude",
			overrideExec: "",
			wantExec:     "claude",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := &Config{Executor: tc.baseExec}
			overlay := &configOverlay{Executor: tc.overrideExec}
			base.applyOverlay(overlay)
			assert.Equal(t, tc.wantExec, base.Executor)
		})
	}
}

func TestApplyOverlay_ClaudeConfig(t *testing.T) {
	base := &Config{
		Claude: ClaudeConfig{
			Flags:           "--verbose",
			ConfigDir:       "/base/dir",
			AnthropicAPIKey: "base-key",
		},
	}

	overlay := &configOverlay{
		Claude: ClaudeConfig{
			Flags: "--model opus",
		},
	}

	base.applyOverlay(overlay)
	assert.Equal(t, "--model opus", base.Claude.Flags)
	assert.Equal(t, "/base/dir", base.Claude.ConfigDir)
	assert.Equal(t, "base-key", base.Claude.AnthropicAPIKey)
}

func TestApplyOverlay_PointerFields(t *testing.T) {
	base := &Config{
		MaxIterations:   50,
		StagnationLimit: 3,
		Timeout:         900,
	}

	zero := 0
	overlay := &configOverlay{
		MaxIterations: &zero, // explicitly set to 0
		// StagnationLimit: nil — not set
	}

	base.applyOverlay(overlay)
	assert.Equal(t, 0, base.MaxIterations)   // overridden to 0
	assert.Equal(t, 3, base.StagnationLimit) // unchanged (nil)
	assert.Equal(t, 900, base.Timeout)       // unchanged (nil)
}

func TestLoadWithDirs_ExecutorConfig(t *testing.T) {
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "PROGRAMMATOR_CLAUDE_FLAGS", "PROGRAMMATOR_ANTHROPIC_API_KEY", "PROGRAMMATOR_EXECUTOR"} {
		saved := os.Getenv(key)
		t.Cleanup(func() { os.Setenv(key, saved) })
		os.Unsetenv(key)
	}

	globalDir := t.TempDir()

	configContent := `
executor: claude
claude:
  flags: "--verbose"
  config_dir: "/custom/dir"
`
	err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(configContent), 0o600)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(globalDir, "")
	require.NoError(t, err)

	assert.Equal(t, "claude", cfg.Executor)
	assert.Equal(t, "--verbose", cfg.Claude.Flags)
	assert.Equal(t, "/custom/dir", cfg.Claude.ConfigDir)
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		executor string
		wantErr  bool
	}{
		{name: "claude is valid", executor: "claude", wantErr: false},
		{name: "empty is valid", executor: "", wantErr: false},
		{name: "unknown is invalid", executor: "gpt", wantErr: true},
		{name: "typo is invalid", executor: "cladue", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Executor: tc.executor}
			err := cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown executor")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
