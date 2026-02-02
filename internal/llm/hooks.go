package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/alexander-akhmetov/programmator/internal/debug"
)

// safePathRe matches paths containing only safe characters.
var safePathRe = regexp.MustCompile(`^[a-zA-Z0-9/_.\-]+$`)

// HookConfig describes hook settings for Claude's --settings flag.
type HookConfig struct {
	PermissionSocketPath string
	GuardMode            bool
}

// BuildHookSettings serialises HookConfig into a JSON string suitable for
// the --settings CLI flag.  Returns empty string when no hooks are needed.
func BuildHookSettings(cfg HookConfig) string {
	var preToolUse []map[string]any

	if cfg.PermissionSocketPath != "" && safePathRe.MatchString(cfg.PermissionSocketPath) {
		hookCmd := fmt.Sprintf("programmator hook --socket %s", cfg.PermissionSocketPath)
		preToolUse = append(preToolUse, map[string]any{
			"matcher": "",
			"hooks": []map[string]any{
				{
					"type":    "command",
					"command": hookCmd,
					"timeout": 120000,
				},
			},
		})
	}

	if cfg.GuardMode {
		home, err := os.UserHomeDir()
		if err != nil {
			debug.Logf("Warning: could not determine home directory for guard mode: %v", err)
		} else {
			dcgConfigPath := filepath.Join(home, ".config", "dcg", "config.toml")
			if !safePathRe.MatchString(dcgConfigPath) {
				debug.Logf("Warning: dcg config path contains unsafe characters, skipping guard mode: %s", dcgConfigPath)
			} else {
				dcgCmd := fmt.Sprintf("DCG_CONFIG='%s' dcg", dcgConfigPath)
				preToolUse = append(preToolUse, map[string]any{
					"matcher": "Bash",
					"hooks": []map[string]any{
						{
							"type":    "command",
							"command": dcgCmd,
							"timeout": 5000,
						},
					},
				})
			}
		}
	}

	if len(preToolUse) == 0 {
		return ""
	}

	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": preToolUse,
		},
	}

	data, err := json.Marshal(settings)
	if err != nil {
		debug.Logf("hooks: failed to marshal settings JSON: %v", err)
		return ""
	}
	return string(data)
}
