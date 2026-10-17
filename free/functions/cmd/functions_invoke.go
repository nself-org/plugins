package main

// Purpose: `nself functions invoke` split out of functions.go (CLI-R12
// Batch B mechanical file-size split). Sends an HTTP request to a deployed
// function and prints the response.
// Inputs: cobra command flags (--payload, --auth, --method) and the
// positional function name.
// Outputs: the HTTP status and (pretty-printed, if JSON) response body on
// stdout; a non-nil error on HTTP >= 400 or request failures. With --json,
// one v1 envelope (data: name, method, status, body) or an error envelope
// (E762 no answer, E763 HTTP >= 400); see envelope.go.
// Constraints: human output unchanged. functionNamePattern and
// functionsCmd (parent) remain in functions.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

var functionsInvokeFlags struct {
	payload string
	auth    string
	method  string
}

var functionsInvokeCmd = &cobra.Command{
	Use:   "invoke <name>",
	Short: "Invoke a deployed function",
	Long: `Send an HTTP request to a deployed function and print the response.

Examples:
  nself functions invoke hello-world
  nself functions invoke hello-world --payload '{"name":"Alice"}'
  nself functions invoke hello-world --method GET`,
	Args: cobra.ExactArgs(1),
	RunE: runFunctionsInvoke,
}

func init() {
	functionsInvokeCmd.Flags().StringVar(&functionsInvokeFlags.payload, "payload", "", "JSON payload body")
	functionsInvokeCmd.Flags().StringVar(&functionsInvokeFlags.auth, "auth", "", "Bearer token for Authorization header")
	functionsInvokeCmd.Flags().StringVar(&functionsInvokeFlags.method, "method", "POST", "HTTP method: GET, POST, PUT, PATCH, DELETE")
	addJSONFlag(functionsInvokeCmd)
}

func runFunctionsInvoke(cmd *cobra.Command, args []string) error {
	r := begin(cmd, false)
	defer r.end()
	name := args[0]
	if !functionNamePattern.MatchString(name) {
		return r.usageSafe(fmt.Sprintf("invalid function name %q", name), "invalid function name")
	}

	port := 3008
	cfg, _, cfgErr := loadHealthConfig()
	if cfgErr == nil && cfg.Functions.Port != 0 {
		port = cfg.Functions.Port
	}

	url := fmt.Sprintf("http://localhost:%d/v1/%s", port, name)
	method := strings.ToUpper(functionsInvokeFlags.method)

	var body io.Reader
	if functionsInvokeFlags.payload != "" {
		body = strings.NewReader(functionsInvokeFlags.payload)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return r.usageSafe(fmt.Sprintf("building request: %v", err), "invalid --method")
	}

	if functionsInvokeFlags.payload != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// Auth token is set but not echoed in logs.
	if functionsInvokeFlags.auth != "" {
		req.Header.Set("Authorization", "Bearer "+functionsInvokeFlags.auth)
	}

	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return r.fail(newErr(fmt.Errorf("invoking %s: %w", name, err), CodeServiceDown, output.ClassInfra,
			"the functions service did not answer",
			"start it with nself start and check the port with nself functions list"))
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return r.fail(newErr(fmt.Errorf("reading response: %w", err), CodeServiceDown, output.ClassInfra,
			"the functions service did not answer",
			"start it with nself start and check the port with nself functions list"))
	}

	fmt.Printf("HTTP %d\n", resp.StatusCode)

	// Pretty-print JSON responses.
	var pretty bytes.Buffer
	if json.Indent(&pretty, respBody, "", "  ") == nil {
		fmt.Println(pretty.String())
	} else {
		fmt.Println(string(respBody))
	}

	if resp.StatusCode >= 400 {
		return r.fail(&fnError{err: fmt.Errorf("function returned HTTP %d", resp.StatusCode), detail: httpDetail(resp.StatusCode)})
	}
	var data any = string(respBody)
	if json.Valid(respBody) {
		data = json.RawMessage(respBody)
	}
	return r.done(map[string]any{"name": name, "method": method, "status": resp.StatusCode, "body": data})
}
