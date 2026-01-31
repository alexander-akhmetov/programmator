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
	assert.Equal(t, 50, cfg.Review.MaxIterations) // Used as base for iteration_pct calculations
	assert.Len(t, cfg.Review.Phases, 3)
	for _, phase := range cfg.Review.Phases {
		assert.True(t, phase.Validate)
	}
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
  max_iterations: 5
  phases:
    - name: custom_phase
      parallel: true
      iteration_limit: 2
      agents:
        - name: custom_agent
          focus:
            - custom focus
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(configContent), 0o600)
	require.NoError(t, err)

	cfg, err := LoadWithDirs(tmpDir, "")
	require.NoError(t, err)

	assert.Equal(t, 5, cfg.Review.MaxIterations)
	require.Len(t, cfg.Review.Phases, 1)
	assert.Equal(t, "custom_phase", cfg.Review.Phases[0].Name)
	assert.True(t, cfg.Review.Phases[0].Parallel)
	assert.Equal(t, 2, cfg.Review.Phases[0].IterationLimit)
	require.Len(t, cfg.Review.Phases[0].Agents, 1)
	assert.Equal(t, "custom_agent", cfg.Review.Phases[0].Agents[0].Name)
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
`)
	cfg, err := parseConfigWithTracking(data)
	require.NoError(t, err)

	assert.True(t, cfg.MaxIterationsSet)
	assert.False(t, cfg.StagnationLimitSet) // not set in YAML
	assert.False(t, cfg.TimeoutSet)         // not set in YAML
}

func TestLoadEmbedded_CodexDefaults(t *testing.T) {
	cfg, err := loadEmbedded()
	require.NoError(t, err)

	assert.True(t, cfg.Codex.Enabled)
	assert.Equal(t, "codex", cfg.Codex.Command)
	assert.Equal(t, "gpt-5.2-codex", cfg.Codex.Model)
	assert.Equal(t, "xhigh", cfg.Codex.ReasoningEffort)
	assert.Equal(t, 3600000, cfg.Codex.TimeoutMs)
	assert.Equal(t, "read-only", cfg.Codex.Sandbox)
	assert.Contains(t, cfg.Codex.ErrorPatterns, "Rate limit")
	assert.Contains(t, cfg.Codex.ErrorPatterns, "quota exceeded")
}

func TestParseConfigWithTracking_Codex(t *testing.T) {
	data := []byte(`
codex:
  enabled: false
  timeout_ms: 600000
`)
	cfg, err := parseConfigWithTracking(data)
	require.NoError(t, err)

	assert.True(t, cfg.Codex.EnabledSet)
	assert.False(t, cfg.Codex.Enabled)
	assert.True(t, cfg.Codex.TimeoutMsSet)
	assert.Equal(t, 600000, cfg.Codex.TimeoutMs)
}

func TestMergeFrom_Codex(t *testing.T) {
	base := &Config{
		Codex: CodexConfig{
			Enabled:   true,
			Command:   "codex",
			Model:     "gpt-5.2-codex",
			TimeoutMs: 3600000,
		},
	}

	override := &Config{
		Codex: CodexConfig{
			Enabled:      false,
			EnabledSet:   true,
			Model:        "gpt-4o",
			TimeoutMs:    600000,
			TimeoutMsSet: true,
		},
	}

	base.mergeFrom(override)

	assert.False(t, base.Codex.Enabled)
	assert.Equal(t, "codex", base.Codex.Command) // not overridden (empty in src)
	assert.Equal(t, "gpt-4o", base.Codex.Model)
	assert.Equal(t, 600000, base.Codex.TimeoutMs)
}

func TestMergeFrom_Codex_ErrorPatterns(t *testing.T) {
	tests := []struct {
		name        string
		basePat     []string
		overridePat []string
		expectedPat []string
	}{
		{
			name:        "nil override does not clear base",
			basePat:     []string{"rate limit"},
			overridePat: nil,
			expectedPat: []string{"rate limit"},
		},
		{
			name:        "non-empty override replaces base",
			basePat:     []string{"rate limit"},
			overridePat: []string{"quota exceeded"},
			expectedPat: []string{"quota exceeded"},
		},
		{
			name:        "empty slice does not clear base",
			basePat:     []string{"rate limit"},
			overridePat: []string{},
			expectedPat: []string{"rate limit"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := &Config{Codex: CodexConfig{ErrorPatterns: tc.basePat}}
			override := &Config{Codex: CodexConfig{ErrorPatterns: tc.overridePat}}
			base.mergeFrom(override)
			assert.Equal(t, tc.expectedPat, base.Codex.ErrorPatterns)
		})
	}
}

func TestMergeFrom_Codex_InvalidReasoningEffort(t *testing.T) {
	base := &Config{Codex: CodexConfig{ReasoningEffort: "high"}}
	override := &Config{Codex: CodexConfig{ReasoningEffort: "invalid"}}
	base.mergeFrom(override)
	assert.Equal(t, "high", base.Codex.ReasoningEffort, "invalid reasoning effort should be ignored")
}

func TestMergeFrom_Codex_InvalidSandbox(t *testing.T) {
	base := &Config{Codex: CodexConfig{Sandbox: "read-only"}}
	override := &Config{Codex: CodexConfig{Sandbox: "invalid"}}
	base.mergeFrom(override)
	assert.Equal(t, "read-only", base.Codex.Sandbox, "invalid sandbox should be ignored")
}

func TestApplyEnv_Codex(t *testing.T) {
	envVars := map[string]string{
		"PROGRAMMATOR_CODEX_ENABLED":          "false",
		"PROGRAMMATOR_CODEX_COMMAND":          "my-codex",
		"PROGRAMMATOR_CODEX_MODEL":            "gpt-4o",
		"PROGRAMMATOR_CODEX_REASONING_EFFORT": "high",
		"PROGRAMMATOR_CODEX_TIMEOUT_MS":       "120000",
		"PROGRAMMATOR_CODEX_SANDBOX":          "network",
	}

	// Save and restore
	saved := make(map[string]string)
	for k := range envVars {
		saved[k] = os.Getenv(k)
	}
	defer func() {
		for k, v := range saved {
			os.Setenv(k, v)
		}
	}()

	for k, v := range envVars {
		os.Setenv(k, v)
	}

	cfg, err := loadEmbedded()
	require.NoError(t, err)
	cfg.applyEnv()

	assert.False(t, cfg.Codex.Enabled)
	assert.True(t, cfg.Codex.EnabledSet)
	assert.Equal(t, "my-codex", cfg.Codex.Command)
	assert.Equal(t, "gpt-4o", cfg.Codex.Model)
	assert.Equal(t, "high", cfg.Codex.ReasoningEffort)
	assert.Equal(t, 120000, cfg.Codex.TimeoutMs)
	assert.True(t, cfg.Codex.TimeoutMsSet)
	assert.Equal(t, "network", cfg.Codex.Sandbox)
}

func TestApplyEnv_Codex_InvalidReasoningEffort(t *testing.T) {
	saved := os.Getenv("PROGRAMMATOR_CODEX_REASONING_EFFORT")
	defer os.Setenv("PROGRAMMATOR_CODEX_REASONING_EFFORT", saved)

	os.Setenv("PROGRAMMATOR_CODEX_REASONING_EFFORT", "superfast")

	cfg, err := loadEmbedded()
	require.NoError(t, err)
	original := cfg.Codex.ReasoningEffort
	cfg.applyEnv()

	assert.Equal(t, original, cfg.Codex.ReasoningEffort, "invalid reasoning effort should be ignored")
}

func TestApplyEnv_Codex_InvalidSandbox(t *testing.T) {
	saved := os.Getenv("PROGRAMMATOR_CODEX_SANDBOX")
	defer os.Setenv("PROGRAMMATOR_CODEX_SANDBOX", saved)

	os.Setenv("PROGRAMMATOR_CODEX_SANDBOX", "unsafe")

	cfg, err := loadEmbedded()
	require.NoError(t, err)
	original := cfg.Codex.Sandbox
	cfg.applyEnv()

	assert.Equal(t, original, cfg.Codex.Sandbox, "invalid sandbox should be ignored")
}

func TestIsValidReasoningEffort(t *testing.T) {
	assert.True(t, isValidReasoningEffort("low"))
	assert.True(t, isValidReasoningEffort("medium"))
	assert.True(t, isValidReasoningEffort("high"))
	assert.True(t, isValidReasoningEffort("xhigh"))
	assert.False(t, isValidReasoningEffort(""))
	assert.False(t, isValidReasoningEffort("superfast"))
}

func TestIsValidSandboxMode(t *testing.T) {
	assert.True(t, isValidSandboxMode("read-only"))
	assert.True(t, isValidSandboxMode("network"))
	assert.True(t, isValidSandboxMode("off"))
	assert.False(t, isValidSandboxMode(""))
	assert.False(t, isValidSandboxMode("unsafe"))
}

func TestApplyEnv_CodexErrorPatterns(t *testing.T) {
	saved := os.Getenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS")
	defer os.Setenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS", saved)

	os.Setenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS", "rate limit,quota exceeded,server error")

	cfg, err := loadEmbedded()
	require.NoError(t, err)
	cfg.applyEnv()

	assert.Equal(t, []string{"rate limit", "quota exceeded", "server error"}, cfg.Codex.ErrorPatterns)
}

func TestApplyEnv_CodexErrorPatterns_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		want      []string
		unchanged bool // if true, patterns should stay as embedded defaults
	}{
		{"empty string", "", nil, true},
		{"whitespace only", "  ,  ", nil, false},
		{"trailing comma", ",rate limit,", []string{"rate limit"}, false},
		{"consecutive commas", "rate limit,,quota", []string{"rate limit", "quota"}, false},
		{"leading comma", ",pattern", []string{"pattern"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := os.Getenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS")
			defer os.Setenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS", saved)

			os.Setenv("PROGRAMMATOR_CODEX_ERROR_PATTERNS", tc.envValue)

			cfg, err := loadEmbedded()
			require.NoError(t, err)
			cfg.applyEnv()

			switch {
			case tc.unchanged:
				// Empty env var means applyEnv doesn't touch the field;
				// embedded defaults remain.
				assert.NotEmpty(t, cfg.Codex.ErrorPatterns)
			case tc.want == nil:
				// Whitespace-only patterns should result in no patterns
				assert.Empty(t, cfg.Codex.ErrorPatterns)
			default:
				assert.Equal(t, tc.want, cfg.Codex.ErrorPatterns)
			}
		})
	}
}

func TestApplyEnv_InvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		envValue string
	}{
		{"invalid max_iterations", "PROGRAMMATOR_MAX_ITERATIONS", "abc"},
		{"invalid timeout", "PROGRAMMATOR_TIMEOUT", "xyz"},
		{"invalid codex_enabled", "PROGRAMMATOR_CODEX_ENABLED", "invalid"},
		{"invalid codex_timeout_ms", "PROGRAMMATOR_CODEX_TIMEOUT_MS", "abc"},
		{"invalid stagnation_limit", "PROGRAMMATOR_STAGNATION_LIMIT", "not_a_number"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := os.Getenv(tc.envKey)
			defer os.Setenv(tc.envKey, saved)

			os.Setenv(tc.envKey, tc.envValue)

			cfg, err := loadEmbedded()
			require.NoError(t, err)

			// Save pre-apply values
			preMaxIter := cfg.MaxIterations
			preTimeout := cfg.Timeout
			preStag := cfg.StagnationLimit
			preCodexEnabled := cfg.Codex.Enabled
			preCodexTimeout := cfg.Codex.TimeoutMs

			cfg.applyEnv()

			// Invalid values should not change the config
			assert.Equal(t, preMaxIter, cfg.MaxIterations)
			assert.Equal(t, preTimeout, cfg.Timeout)
			assert.Equal(t, preStag, cfg.StagnationLimit)
			assert.Equal(t, preCodexEnabled, cfg.Codex.Enabled)
			assert.Equal(t, preCodexTimeout, cfg.Codex.TimeoutMs)
		})
	}
}

func TestApplyEnv_CodexCommandValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantSet bool
	}{
		{"valid simple name", "my-codex", true},
		{"valid with dots", "codex.v2", true},
		{"invalid with slash", "/usr/bin/codex", false},
		{"invalid with space", "codex cmd", false},
		{"invalid with semicolon", "codex;rm", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := os.Getenv("PROGRAMMATOR_CODEX_COMMAND")
			defer os.Setenv("PROGRAMMATOR_CODEX_COMMAND", saved)

			os.Setenv("PROGRAMMATOR_CODEX_COMMAND", tc.value)

			cfg, err := loadEmbedded()
			require.NoError(t, err)
			cfg.applyEnv()

			if tc.wantSet {
				assert.Equal(t, tc.value, cfg.Codex.Command)
			} else {
				assert.NotEqual(t, tc.value, cfg.Codex.Command)
			}
		})
	}
}

func TestIsValidCommandName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "codex", true},
		{"with dashes", "my-codex", true},
		{"with dots", "codex.v2", true},
		{"with underscore", "my_codex", true},
		{"empty", "", false},
		{"with slash", "/usr/bin/codex", false},
		{"with space", "codex cmd", false},
		{"with semicolon", "codex;rm", false},
		{"with ampersand", "codex&&echo", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isValidCommandName(tc.input))
		})
	}
}

func TestApplyEnv_CodexModelValidation(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantSet bool
	}{
		{"valid model", "gpt-4o", true},
		{"valid with dots", "gpt-5.2-codex", true},
		{"valid with colon", "org:model", true},
		{"invalid with slash", "path/to/model", false},
		{"invalid with space", "gpt 4o", false},
		{"invalid with semicolon", "model;rm", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			saved := os.Getenv("PROGRAMMATOR_CODEX_MODEL")
			defer os.Setenv("PROGRAMMATOR_CODEX_MODEL", saved)

			os.Setenv("PROGRAMMATOR_CODEX_MODEL", tc.value)

			cfg, err := loadEmbedded()
			require.NoError(t, err)
			cfg.applyEnv()

			if tc.wantSet {
				assert.Equal(t, tc.value, cfg.Codex.Model)
			} else {
				assert.NotEqual(t, tc.value, cfg.Codex.Model)
			}
		})
	}
}

func TestMergeFrom_CodexModelValidation(t *testing.T) {
	base := &Config{
		Codex: CodexConfig{Model: "gpt-5.2-codex"},
	}
	override := &Config{
		Codex: CodexConfig{Model: "invalid/model"},
	}

	base.mergeFrom(override)
	assert.Equal(t, "gpt-5.2-codex", base.Codex.Model, "invalid model name should not override base")
}

func TestParseConfigWithTrackingIgnoresReviewPasses(t *testing.T) {
	data := []byte(`
review:
  passes: []
`)
	cfg, err := parseConfigWithTracking(data)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
}
