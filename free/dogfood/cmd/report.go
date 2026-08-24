// Purpose: `nself-dogfood report` — shows the most recently saved audit
// report without re-running the checks. Unchanged from the pre-extraction
// `nself dogfood report`.
//
// Inputs: --json (machine output); reads from .nself/dogfood/ under the
// current working directory.
//
// Outputs: the latest report's summary and any non-passing checks.
//
// Constraints: returns a plain error (not a special exit code) when no
// report exists yet — this always meant "run `nself dogfood audit` first",
// never a fail/warn verdict of its own.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-dogfood/internal/dogfood"
	"github.com/nself-org/nself-dogfood/internal/tui"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show the latest dogfood audit report",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		cwd, _ := os.Getwd()

		report, err := dogfood.GetLatestReport(cwd)
		if err != nil {
			return fmt.Errorf("no reports found: %w", err)
		}

		if jsonOut {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		tui.CommandHeader("Latest Dogfood Report", report.RanAt.Format("2006-01-02 15:04:05"))
		tui.Bullet(fmt.Sprintf("Pass: %d  Fail: %d  Warn: %d  Skip: %d",
			report.PassCount, report.FailCount, report.WarnCount, report.SkipCount))

		for _, c := range report.Checks {
			if c.Status != "pass" {
				tui.PrintCheck(c.Status, c.Name, c.Message)
			}
		}

		return nil
	},
}

func init() {
	reportCmd.Flags().Bool("json", false, "JSON output")
}
