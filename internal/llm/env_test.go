package llm

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterEnv(t *testing.T) {
	tests := []struct {
		name     string
		environ  []string
		prefixes []string
		want     []string
	}{
		{
			name:     "removes matching prefixes",
			environ:  []string{"FOO=1", "BAR=2", "BAZ=3"},
			prefixes: []string{"BAR="},
			want:     []string{"FOO=1", "BAZ=3"},
		},
		{
			name:     "removes multiple prefixes",
			environ:  []string{"A=1", "B=2", "C=3"},
			prefixes: []string{"A=", "C="},
			want:     []string{"B=2"},
		},
		{
			name:     "no prefixes keeps all",
			environ:  []string{"A=1", "B=2"},
			prefixes: nil,
			want:     []string{"A=1", "B=2"},
		},
		{
			name:     "empty environ returns empty",
			environ:  nil,
			prefixes: []string{"A="},
			want:     []string{},
		},
		{
			name:     "prefix must match start of entry",
			environ:  []string{"XFOO=1", "FOO=2"},
			prefixes: []string{"FOO="},
			want:     []string{"XFOO=1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterEnv(tc.environ, tc.prefixes...)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestProviderAPIKeyEnvVars(t *testing.T) {
	assert.Equal(t, "ANTHROPIC_API_KEY", ProviderAPIKeyEnvVars["anthropic"])
	assert.Equal(t, "OPENAI_API_KEY", ProviderAPIKeyEnvVars["openai"])
	assert.Equal(t, "GEMINI_API_KEY", ProviderAPIKeyEnvVars["google"])
	assert.Equal(t, "GROQ_API_KEY", ProviderAPIKeyEnvVars["groq"])
	assert.Equal(t, "MISTRAL_API_KEY", ProviderAPIKeyEnvVars["mistral"])
	assert.Len(t, ProviderAPIKeyEnvVars, 5)
}

func TestAllProviderAPIKeyPrefixes(t *testing.T) {
	prefixes := AllProviderAPIKeyPrefixes()
	require.Len(t, prefixes, 5)

	// Sort for deterministic comparison since map iteration is non-deterministic.
	sort.Strings(prefixes)
	expected := []string{
		"ANTHROPIC_API_KEY=",
		"GEMINI_API_KEY=",
		"GROQ_API_KEY=",
		"MISTRAL_API_KEY=",
		"OPENAI_API_KEY=",
	}
	require.Equal(t, expected, prefixes)
}
