// Purpose: `nself-dogfood audit` — runs the full dogfood check list and
// prints or saves the result. Behavior is unchanged from the pre-extraction
// `nself dogfood audit`: same flags, same exit codes, same section-grouped
// output.
//
// Inputs: --json (machine output), --save (write to .nself/dogfood/,
// defaults on), and the current working directory as the project root.
//
// Outputs: a human or JSON report on stdout; the process exit code alone
// conveys pass/warn/fail (0/2/1) since the report body already states the
// counts.
//
// Constraints: exit codes must stay 0/1/2 exactly as documented in the
// command's own --help, because scripts (including CI dogfood gates) may
// already depend on them.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-dogfood/internal/dogfood"
	"github.com/nself-org/nself-dogfood/internal/tui"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run comprehensive dogfood audit (21 checks)",
	Long: `Run the complete dogfood audit checklist against your production environment.

Verifies backups, DR, tenancy, licensing, secrets, migrations, monitoring,
security, watchdog, queue health, and more.

Exit codes:
  0  All checks passed
  1  One or more failures
  2  Warnings only`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		save, _ := cmd.Flags().GetBool("save")

		cwd, _ := os.Getwd()
		ctx := cmd.Context()

		report := dogfood.RunAudit(ctx, cwd)

		if save {
			if err := dogfood.SaveReport(cwd, report); err != nil {
				tui.Warn(fmt.Sprintf("Failed to save report: %v", err))
			} else {
				tui.Bullet("Report saved to .nself/dogfood/")
			}
		}

		if jsonOut {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
		} else {
			tui.CommandHeader("Dogfood Audit", fmt.Sprintf("%d checks", len(report.Checks)))

			currentSection := ""
			for _, c := range report.Checks {
				if c.Section != currentSection {
					currentSection = c.Section
					tui.Section(currentSection)
				}
				tui.PrintCheck(c.Status, c.Name, c.Message)
			}

			fmt.Println()
			tui.Separator()
			tui.Bullet(fmt.Sprintf("Pass: %d  Fail: %d  Warn: %d  Skip: %d",
				report.PassCount, report.FailCount, report.WarnCount, report.SkipCount))
		}

		// The report above is the output; exit status alone conveys the verdict.
		if report.FailCount > 0 {
			exitCode = 1
			return nil
		}
		if report.WarnCount > 0 {
			exitCode = 2
			return nil
		}
		return nil
	},
}

func init() {
	auditCmd.Flags().Bool("json", false, "JSON output")
	auditCmd.Flags().Bool("save", true, "Save report to .nself/dogfood/")
}
