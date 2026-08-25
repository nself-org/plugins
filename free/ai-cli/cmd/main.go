package main

import (
	"github.com/spf13/cobra"
	"os"
	"strings"
)

// -----------------------------------------------------------------------------
// `nself ai` root + `nself ai local` subtree
// Sprint P88-S02: zero-config local LLM automation.
// -----------------------------------------------------------------------------

var aiCmd = &cobra.Command{
	Use:   "ai",
	Short: "Manage the nSelf AI plugin and local LLM stack",
	Long: `Commands for managing the nSelf AI plugin (providers, models, routing).

Subcommands:
  local    Manage the local Ollama runtime and models`,
	RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

var aiLocalCmd = &cobra.Command{
	Use:   "local",
	Short: "Manage local Ollama runtime and models",
	Long: `Install, inspect, and manage a local Ollama runtime for zero-config AI.

Subcommands:
  install     Install Ollama, systemd service, firewall, and recommended models
  models      List / add / remove / recommend local models
  health      Show Ollama + plugin-ai health
  swap        Hot-swap the default model for a task
  benchmark   Run benchmark prompts against one or more models`,
	RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
}

// -----------------------------------------------------------------------------
// `nself ai local install`  (T-02-03)
// -----------------------------------------------------------------------------

func init() {
	// install flags
	aiLocalInstallCmd.Flags().BoolVar(&aiInstallYes, "yes", false, "Non-interactive mode")
	aiLocalInstallCmd.Flags().BoolVar(&aiInstallNoModels, "no-models", false, "Skip model pulls")
	aiLocalInstallCmd.Flags().StringVar(&aiInstallModelFlag, "model", "", "Pull only this model")
	aiLocalInstallCmd.Flags().StringVar(&aiInstallBind, "bind", "", "host:port to bind Ollama to")
	aiLocalInstallCmd.Flags().BoolVar(&aiInstallJSON, "json", false, "Emit JSON output")

	// models flags
	aiLocalModelsListCmd.Flags().BoolVar(&modelsListInstalled, "installed", false, "Show only installed")
	aiLocalModelsListCmd.Flags().BoolVar(&modelsListRegistered, "registered", false, "Show only registered")
	aiLocalModelsListCmd.Flags().BoolVar(&modelsListJSON, "json", false, "Emit JSON output")
	aiLocalModelsAddCmd.Flags().StringVar(&modelAddTask, "task", "chat", "Comma-separated task classes")
	aiLocalModelsAddCmd.Flags().BoolVar(&modelAddDefault, "default", false, "Set as default for the tasks")
	aiLocalModelsRemoveCmd.Flags().BoolVar(&modelRemoveForce, "force", false, "Remove even if default")
	aiLocalModelsRecommendCmd.Flags().StringVar(&modelRecommendTier, "tier", "auto", "Force a tier (auto|minimal|balanced|max)")

	aiLocalModelsCmd.AddCommand(aiLocalModelsListCmd)
	aiLocalModelsCmd.AddCommand(aiLocalModelsAddCmd)
	aiLocalModelsCmd.AddCommand(aiLocalModelsRemoveCmd)
	aiLocalModelsCmd.AddCommand(aiLocalModelsRecommendCmd)

	// health/swap/benchmark
	aiLocalHealthCmd.Flags().BoolVar(&aiHealthWatch, "watch", false, "Re-poll every 2s")
	aiLocalHealthCmd.Flags().BoolVar(&aiHealthJSON, "json", false, "Emit JSON output")
	aiLocalSwapCmd.Flags().StringVar(&swapTask, "task", "chat", "Task: chat|embed|classify|all")
	aiLocalSwapCmd.Flags().StringVar(&swapReason, "reason", "", "Free-text reason (audit log)")
	aiLocalBenchmarkCmd.Flags().StringVar(&benchTasks, "tasks", "chat", "Comma-separated tasks")
	aiLocalBenchmarkCmd.Flags().IntVar(&benchIterations, "iterations", 5, "Iterations per task")

	aiLocalCmd.AddCommand(aiLocalInstallCmd)
	aiLocalCmd.AddCommand(aiLocalModelsCmd)
	aiLocalCmd.AddCommand(aiLocalHealthCmd)
	aiLocalCmd.AddCommand(aiLocalSwapCmd)
	aiLocalCmd.AddCommand(aiLocalBenchmarkCmd)

	aiCmd.AddCommand(aiLocalCmd)
}

func main() {
	prefixUsageWithNself(aiCmd)

	// Cobra default Args validator (legacyArgs) rejects an unrecognised
	// first argument only for a ROOT command with subcommands; for a child
	// it passes them to RunE. Inside the CLI this command was a child, so
	// `nself ai nosuch` printed help. As a root it errors instead,
	// with a message naming a binary the user never typed. ArbitraryArgs
	// restores the child behaviour.
	aiCmd.Args = cobra.ArbitraryArgs

	aiCmd.CompletionOptions.DisableDefaultCmd = true
	aiCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	aiCmd.SilenceUsage = true

	if err := aiCmd.Execute(); err != nil {
		// cobra has already printed the error; exit non-zero without repeating
		// it. The CLI mirrors this status and stays silent, so the plugin's own
		// message is the only one the user sees.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Applied to the root; cobra passes templates
// down to subcommands, so one call covers the whole tree and
// `nself ai <sub> --help` renders correctly too.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
