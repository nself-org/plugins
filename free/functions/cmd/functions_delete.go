package main

// Purpose: `nself functions delete` split out of functions.go (CLI-R12
// Batch B mechanical file-size split). Removes a deployed function's
// directory from ./functions/<name>/.
// Inputs: cobra command flags (--confirm) and the positional function name.
// Outputs: stdout confirmation on success; errors when --confirm is
// missing or the function directory does not exist. With --json, one v1
// envelope (data: name, deleted) or an error envelope; see envelope.go.
// Constraints: human output unchanged. functionNamePattern and
// functionsCmd (parent) remain in functions.go.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

var functionsDeleteFlags struct {
	confirm bool
}

var functionsDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a deployed function",
	Long: `Remove a deployed function and its directory from ./functions/<name>/.

Requires --confirm to prevent accidental deletion.

Example:
  nself functions delete hello-world --confirm`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsDelete,
}

func init() {
	functionsDeleteCmd.Flags().BoolVar(&functionsDeleteFlags.confirm, "confirm", false, "Confirm deletion (required)")
	addJSONFlag(functionsDeleteCmd)
}

func runFunctionsDelete(cmd *cobra.Command, args []string) error {
	r := begin(cmd, false)
	defer r.end()
	name := args[0]
	if !functionNamePattern.MatchString(name) {
		return r.usageSafe(fmt.Sprintf("invalid function name %q", name), "invalid function name")
	}

	if !functionsDeleteFlags.confirm {
		return r.fail(newErr(fmt.Errorf("--confirm is required to delete a function"),
			CodeBlocked, output.ClassDestructiveBlocked,
			"delete refused: --confirm was not given, the function was not deleted",
			"pass --confirm to delete the function"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return r.fail(fileOpErr(fmt.Errorf("getting working directory: %w", err)))
	}

	fnDir := filepath.Join(cwd, "functions", name)
	if _, err := os.Stat(fnDir); os.IsNotExist(err) {
		return r.fail(newErr(fmt.Errorf("function %q not found at %s", name, fnDir),
			CodeNotFound, output.ClassUser, "the function was not found",
			"check the name with nself functions list"))
	}

	if err := os.RemoveAll(fnDir); err != nil {
		return r.fail(fileOpErr(fmt.Errorf("deleting function directory: %w", err)))
	}

	fmt.Printf("Function %q deleted.\n", name) // stderr in JSON mode (begin)
	return r.done(map[string]any{"name": name, "deleted": true})
}
