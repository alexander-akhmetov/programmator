package tui

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alexander-akhmetov/programmator/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage programmator configuration",
	Long:  `View and manage programmator configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show resolved configuration with source annotations",
	Long: `Show the fully resolved configuration with annotations indicating
where each value came from.

Configuration is loaded from multiple sources with the following precedence:
  1. Embedded defaults (built into binary)
  2. Global config (~/.config/programmator/config.yaml)
  3. Environment variables
  4. Local config (.programmator/config.yaml)
  5. CLI flags (highest precedence)`,
	RunE: runConfigShow,
}

func init() {
	configCmd.AddCommand(configShowCmd)
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("# Programmator Configuration")
	fmt.Println()
	fmt.Println("## Sources (in order of precedence)")
	for _, src := range cfg.Sources() {
		fmt.Printf("  - %s\n", src)
	}
	fmt.Println()

	fmt.Println("## Directories")
	fmt.Printf("  Global config: %s\n", cfg.ConfigDir())
	if cfg.LocalDir() != "" {
		fmt.Printf("  Local config:  %s\n", cfg.LocalDir())
	} else {
		fmt.Printf("  Local config:  (none detected)\n")
	}
	fmt.Println()

	fmt.Println("## Loop Settings")
	fmt.Printf("  max_iterations:   %d\n", cfg.MaxIterations)
	fmt.Printf("  stagnation_limit: %d\n", cfg.StagnationLimit)
	fmt.Printf("  timeout:          %ds\n", cfg.Timeout)
	fmt.Println()

	fmt.Println("## Ticket Settings")
	fmt.Printf("  ticket_command: %s\n", cfg.TicketCommand)
	fmt.Println()

	fmt.Println("## Executor Settings")
	fmt.Printf("  executor: %s\n", cfg.Executor)
	if cfg.PlanExecutor != "" {
		fmt.Printf("  plan_executor: %s\n", cfg.PlanExecutor)
	} else {
		fmt.Printf("  plan_executor: (inherits executor)\n")
	}
	fmt.Println()

	fmt.Println("## Claude Settings")
	if cfg.Claude.Flags != "" {
		fmt.Printf("  flags:            %s\n", cfg.Claude.Flags)
	} else {
		fmt.Printf("  flags:            (none)\n")
	}
	if cfg.Claude.ConfigDir != "" {
		fmt.Printf("  config_dir:       %s\n", cfg.Claude.ConfigDir)
	} else {
		fmt.Printf("  config_dir:       (default)\n")
	}
	if cfg.Claude.AnthropicAPIKey != "" {
		fmt.Printf("  anthropic_api_key: (set)\n")
	} else {
		fmt.Printf("  anthropic_api_key: (not set)\n")
	}
	fmt.Println()

	fmt.Println("## Review Settings")
	fmt.Printf("  max_iterations: %d\n", cfg.Review.MaxIterations)
	fmt.Printf("  parallel:       %t\n", cfg.Review.Parallel)
	if len(cfg.Review.Agents) > 0 {
		fmt.Println("  agents:")
		for _, agent := range cfg.Review.Agents {
			fmt.Printf("    - %s\n", agent.Name)
			if agent.Executor != "" {
				fmt.Printf("        executor: %s\n", agent.Executor)
			}
			if len(agent.Focus) > 0 {
				fmt.Printf("        focus: %s\n", strings.Join(agent.Focus, ", "))
			}
		}
	}

	return nil
}
