package main

// Purpose: `nself functions list` split out of functions.go (CLI-R12
// Batch B mechanical file-size split), plus its health-probe helper.
// Enumerates ./functions/ subdirectories and reports liveness for each.
// Inputs: cobra command flags (--json, --runtime).
// Outputs: a table or JSON array of FunctionInfo; errors wrap directory
// read failures. With --json: v1.5 prints one v1 envelope (data.functions);
// v1.4 keeps the pre-contract bytes (bare array, or plain text when there is
// nothing to list); see envelope.go.
// Constraints: human output unchanged. functionsCmd (parent)
// remains in functions.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/compat"
)

var functionsListFlags struct {
	jsonOut bool
	runtime string
}

// FunctionInfo describes a deployed function.
type FunctionInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	URL    string `json:"url"`
	Dir    string `json:"dir"`
}

var functionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployed functions",
	RunE:  runFunctionsList,
}

func init() {
	functionsListCmd.Flags().BoolVar(&functionsListFlags.jsonOut, "json", false, "Output as JSON")
	functionsListCmd.Flags().StringVar(&functionsListFlags.runtime, "runtime", "", "Filter by runtime")
}

func runFunctionsList(cmd *cobra.Command, args []string) error {
	// compat.V15(P7-SURF-31): --json prints the bare FunctionInfo array (plain text when empty) -> one v1 envelope
	r := begin(cmd, !compat.V15())
	defer r.end()

	fns, noDir, err := scanFunctions(cmd)
	if err != nil {
		return r.fail(err)
	}
	if r.json {
		return r.done(map[string]any{"functions": fns})
	}
	if noDir {
		fmt.Println("No functions deployed. Use `nself functions deploy` to deploy one.")
		return nil
	}
	if len(fns) == 0 {
		fmt.Println("No functions found in ./functions/")
		return nil
	}

	if functionsListFlags.jsonOut {
		// compat.V15(P7-SURF-31): bare indented array (v1.4 bytes) -> envelope above
		data, err := json.MarshalIndent(fns, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%-24s  %-10s  %s\n", "NAME", "STATUS", "URL")
	for _, fn := range fns {
		fmt.Printf("%-24s  %-10s  %s\n", fn.Name, fn.Status, fn.URL)
	}
	return nil
}

// scanFunctions enumerates ./functions/ and probes each function's health.
// noDir is true when ./functions/ does not exist. fns is never nil.
func scanFunctions(cmd *cobra.Command) (fns []FunctionInfo, noDir bool, err error) {
	fns = []FunctionInfo{}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, false, fileOpErr(fmt.Errorf("getting working directory: %w", err))
	}

	fnDir := filepath.Join(cwd, "functions")
	entries, err := os.ReadDir(fnDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fns, true, nil
		}
		return nil, false, fileOpErr(fmt.Errorf("reading functions directory: %w", err))
	}

	// Determine functions service base URL and project name for status probing.
	port := 3008
	cfg, _, cfgErr := loadHealthConfig()
	if cfgErr == nil && cfg.Functions.Port != 0 {
		port = cfg.Functions.Port
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		info := FunctionInfo{
			Name: name,
			Dir:  filepath.Join(fnDir, name),
			URL:  fmt.Sprintf("http://localhost:%d/v1/%s", port, name),
		}

		// Probe liveness (best-effort, non-fatal).
		probeCtx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
		info.Status = probeFunctionHealth(probeCtx, port, name)
		cancel()

		fns = append(fns, info)
	}
	return fns, false, nil
}

// probeFunctionHealth probes GET /v1/<name>/healthz and returns "healthy" or "unknown".
func probeFunctionHealth(ctx context.Context, port int, name string) string {
	url := fmt.Sprintf("http://localhost:%d/v1/%s/healthz", port, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "unknown"
	}
	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return "unreachable"
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 500 {
		return "healthy"
	}
	return "unhealthy"
}
