// Purpose: shared client/license/output helpers and error mapping for the
// mail plugin's subcommands, moved verbatim from the core CLI's
// cmd/commands/mail.go under CLI-R11. Behavior is unchanged; only the
// package name and the internal/license + internal/plugin replacements
// (internal/licensekeys + main.go's local exitCodeError) changed.
//
// Inputs: environment variables (NSELF_PING_API_URL, license key env vars),
// the stored license key file.
//
// Outputs: a configured *mail.Client, or a requireLicense error carrying
// exit code 2 when no license key is configured.
//
// Constraints: no dependency on the core CLI's internal/* packages.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nself-org/nself-mail/internal/licensekeys"
	"github.com/nself-org/nself-mail/internal/mail"
)

// mailExitNoLicense is the process exit code when no license key is configured.
const mailExitNoLicense = 2

// resolveMailClient picks the ping_api base URL from env (NSELF_PING_API_URL,
// default https://ping.nself.org) and the first configured license key from
// internal/licensekeys.CollectLicenseKeys (which already reads
// NSELF_LICENSE_KEY, NSELF_PLUGIN_LICENSE_KEY, and NSELF_LICENSE_KEY_1..10).
//
// Returns (nil, nil) when no license is configured so callers can emit the
// canonical "requires nSelf+ or nClaw bundle" message and exit with code 2.
func resolveMailClient() (*mail.Client, error) {
	pingURL := os.Getenv("NSELF_PING_API_URL")
	if pingURL == "" {
		pingURL = mail.DefaultPingURL
	}
	keys := licensekeys.CollectLicenseKeys()
	if len(keys) == 0 {
		return nil, nil
	}
	return mail.New(pingURL, keys[0]), nil
}

// requireLicense prints the canonical "no license" message to stderr and
// returns a *exitCodeError so main() exits with code 2. main()
// short-circuits on exitCodeError without printing, so we print here.
func requireLicense(cmd *cobra.Command) error {
	fmt.Fprintln(cmd.ErrOrStderr(), "Error: nself mail requires nSelf+ or nClaw bundle (Postmark plugin) — run 'nself license add <key>'")
	return &exitCodeError{Code: mailExitNoLicense}
}

// printResult emits either JSON or a human-readable rendering. The renderer
// is invoked only when --json is false. If renderer is nil, the value is
// printed as a key:value block via reflection-free JSON marshal indent.
func printResult(jsonMode bool, v interface{}, render func()) error {
	if jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	if render != nil {
		render()
	}
	return nil
}

// ── error mapping ─────────────────────────────────────────────────────────

// mapMailError converts mail-package sentinel errors into user-readable
// cobra errors. 4xx → short message, 5xx → flagged as ping_api issue, network
// → guidance to check connectivity.
func mapMailError(err error) error {
	switch {
	case errors.Is(err, mail.ErrUnauthorized):
		return errors.New("license rejected by ping.nself.org — run 'nself license validate'")
	case errors.Is(err, mail.ErrUnreachable):
		return errors.New("ping.nself.org unreachable — check connectivity")
	case errors.Is(err, mail.ErrServer):
		return fmt.Errorf("ping_api server error: %w", err)
	default:
		return err
	}
}
