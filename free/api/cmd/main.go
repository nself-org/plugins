package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/nself-api/internal/ui"

	"github.com/spf13/cobra"
)

// apiVersionRow represents one surface's version info for the api version command.
type apiVersionRow struct {
	Surface    string `json:"surface"`
	Version    string `json:"version"`
	Deprecated bool   `json:"deprecated"`
	EOLDate    string `json:"eol_date,omitempty"`
}

// apiCmd is the root command for API versioning operator tooling.
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "API versioning and deprecation tooling for operators",
	Long: `Operator tooling for the nSelf API versioning baseline (v1.0.9 LTS).

The nSelf LTS contract guarantees backward compatibility through 2027-04-17.
These commands let you measure and verify that commitment on your install.`,
}

// apiVersionCmd reports the API version observable from this install.
var apiVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show API version for every surface observable from this install",
	Long: `Show the API version for every surface reachable from this install:

  - CLI binary version (this binary)
  - ping_api version (probed via HTTP)
  - Marketplace Worker version (probed via HTTP)
  - Per-installed-plugin SDK version (from plugin.json api_version if declared)
  - Hasura schema version (if nself is running locally)

Deprecation status is cross-referenced against the central deprecation registry.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		surface, _ := cmd.Flags().GetString("surface")
		timeout, _ := cmd.Flags().GetInt("timeout")

		rows := collectAPIVersions(surface, timeout)

		if jsonOut {
			return ui.PrintJSON(rows)
		}

		fmt.Printf("\n%-30s %-15s %-12s %s\n", "Surface", "Version", "Deprecated", "EOL Date")
		fmt.Println(strings.Repeat("-", 72))
		for _, row := range rows {
			dep := "no"
			if row.Deprecated {
				dep = "YES"
			}
			eol := row.EOLDate
			if eol == "" {
				eol = "-"
			}
			fmt.Printf("%-30s %-15s %-12s %s\n", row.Surface, row.Version, dep, eol)
		}
		fmt.Println()
		return nil
	},
}

func init() {
	apiCmd.AddCommand(apiVersionCmd)
	apiCmd.AddCommand(apiDeprecationCheckCmd)
	apiCmd.AddCommand(apiChangelogCmd)

	// api version flags
	apiVersionCmd.Flags().Bool("json", false, "Output as JSON")
	apiVersionCmd.Flags().String("surface", "", "Filter to a single surface (cli, ping_api, marketplace, sdk, hasura)")
	apiVersionCmd.Flags().Int("timeout", 5, "HTTP probe timeout in seconds")

	// api deprecation-check flags (G6: --plugin, --strict for G11)
	apiDeprecationCheckCmd.Flags().Bool("json", false, "Output as JSON")
	apiDeprecationCheckCmd.Flags().String("plugin", "", "Check a specific plugin by name (e.g. --plugin ai)")
	apiDeprecationCheckCmd.Flags().Bool("strict", false, "Exit 1 if any BREAKING entries exist (used by CI gate, G11)")

	// api changelog flags
	apiChangelogCmd.Flags().Bool("json", false, "Output as JSON")

}

func main() {
	prefixUsageWithNself(apiCmd)

	// Cobra default Args validator (legacyArgs) rejects an unrecognised
	// first argument only for a ROOT command with subcommands; for a child
	// it passes them to RunE. Inside the CLI this command was a child, so
	// `nself api nosuch` printed help. As a root it errors instead,
	// with a message naming a binary the user never typed. ArbitraryArgs
	// restores the child behaviour.
	apiCmd.Args = cobra.ArbitraryArgs

	apiCmd.CompletionOptions.DisableDefaultCmd = true
	apiCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	apiCmd.SilenceUsage = true

	if err := apiCmd.Execute(); err != nil {
		// cobra has already printed the error; exit non-zero without repeating
		// it. The CLI mirrors this status and stays silent, so the plugin's own
		// message is the only one the user sees.
		os.Exit(1)
	}
}

// prefixUsageWithNself rewrites the usage template so every rendered command
// path is preceded by "nself ". Applied to the root; cobra passes templates
// down to subcommands, so one call covers the whole tree and
// `nself api <sub> --help` renders correctly too.
func prefixUsageWithNself(root *cobra.Command) {
	root.SetUsageTemplate(strings.NewReplacer(
		"{{.CommandPath}}", "nself {{.CommandPath}}",
		"{{.UseLine}}", "nself {{.UseLine}}",
	).Replace(root.UsageTemplate()))
}
