package main

// Purpose: "nself claw unlock" and "nself claw setup". Unlock asks the claw
// server for a one-time unlock token and prints it once; setup posts that
// token with a display name and a password to /claw/auth/setup to create the
// first account (the server requires unlock_token since bundles#343).
// Inputs: cobra command/args, --minutes, --token, --name, --password-stdin,
// and the password on stdin. Outputs: the token (unlock) or a created-account
// line (setup). The password is never a flag or env var and is never printed.
// Constraints: moved from claw_pair.go (file size); the setup command is
// registered from this file's init so claw.go stays unchanged.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nself-org/nself-claw/internal/httptimeout"
	"github.com/spf13/cobra"
)

var (
	clawSetupToken         string
	clawSetupName          string
	clawSetupPasswordStdin bool
)

var clawSetupCmd = &cobra.Command{
	Use:   "setup --token T --name N --password-stdin",
	Short: "Create the first claw account with the token from 'nself claw unlock'",
	Long: `Create the first nClaw account. Run 'nself claw unlock' on the server first,
then pass the token it printed within the unlock window.

The password is read from stdin (one line) and is never accepted as a flag or
environment variable.

Example:
  printf '%s\n' "$PASSWORD" | nself claw setup --token <token> --name Ali --password-stdin`,
	Args: cobra.NoArgs,
	RunE: runClawSetup,
}

func init() {
	clawSetupCmd.Flags().StringVar(&clawSetupToken, "token", "", "Unlock token printed by 'nself claw unlock'")
	clawSetupCmd.Flags().StringVar(&clawSetupName, "name", "", "Display name for the first account")
	clawSetupCmd.Flags().BoolVar(&clawSetupPasswordStdin, "password-stdin", false, "Read the password from stdin")
	clawCmd.AddCommand(clawSetupCmd)
}

// clawInternalURL returns the claw plugin base URL.
func clawInternalURL() string {
	if u := os.Getenv("PLUGIN_CLAW_INTERNAL_URL"); u != "" {
		return u
	}
	return "http://claw:3710"
}

// serverErrorMessage reads {"error": "..."} from a failed response.
func serverErrorMessage(resp *http.Response) string {
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if msg := body["error"]; msg != "" {
		return msg
	}
	return fmt.Sprintf("HTTP %d", resp.StatusCode)
}

func runClawUnlock(cmd *cobra.Command, args []string) error {
	if clawUnlockMinutes < 1 || clawUnlockMinutes > 60 {
		return fmt.Errorf("--minutes must be between 1 and 60")
	}

	payload, err := json.Marshal(map[string]int{"duration_minutes": clawUnlockMinutes})
	if err != nil {
		return fmt.Errorf("marshal unlock request: %w", err)
	}

	req, err := http.NewRequestWithContext(cmd.Context(), "POST", clawInternalURL()+"/claw/auth/unlock", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("create unlock request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return fmt.Errorf("unlock request failed: %w\nMake sure the claw plugin is running (nself plugin install claw && nself start)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("unlock rejected: this command must be run on the server (localhost only)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unlock failed: %s", serverErrorMessage(resp))
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result.Token == "" {
		return fmt.Errorf("unlock failed: server returned no token")
	}

	expiresDisplay := result.ExpiresAt
	if t, err := time.Parse(time.RFC3339, result.ExpiresAt); err == nil {
		expiresDisplay = t.Local().Format("15:04:05")
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  nClaw unlocked for %d minutes.\n", clawUnlockMinutes)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Unlock token: %s\n", result.Token)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Create the first account with:")
	fmt.Fprintln(out, "    nself claw setup --token <token> --name <name> --password-stdin")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  The token is shown once. It expires at %s or after first use.\n", expiresDisplay)
	fmt.Fprintln(out)
	return nil
}

func runClawSetup(cmd *cobra.Command, args []string) error {
	if clawSetupToken == "" || strings.TrimSpace(clawSetupName) == "" {
		return fmt.Errorf("--token and --name are required")
	}
	if !clawSetupPasswordStdin {
		return fmt.Errorf("--password-stdin is required: the password is read from stdin, never from a flag")
	}

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading password from stdin: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return fmt.Errorf("no password on stdin")
	}

	payload, err := json.Marshal(map[string]string{
		"unlock_token": clawSetupToken,
		"display_name": clawSetupName,
		"password":     password,
	})
	if err != nil {
		return fmt.Errorf("marshal setup request: %w", err)
	}

	req, err := http.NewRequestWithContext(cmd.Context(), "POST", clawInternalURL()+"/claw/auth/setup", strings.NewReader(string(payload)))
	if err != nil {
		return fmt.Errorf("create setup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return fmt.Errorf("setup request failed: %w\nMake sure the claw plugin is running (nself plugin install claw && nself start)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("setup failed: %s", serverErrorMessage(resp))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Account created for %q. Sign in with that name and the password you piped in.\n", clawSetupName)
	return nil
}
