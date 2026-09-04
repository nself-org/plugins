// Purpose: shared client/output helpers and error mapping for the mail
// plugin's subcommands, moved verbatim from the core CLI's
// cmd/commands/mail.go under CLI-R11. mail is a free plugin (plugin.json:
// requires_license=false, isCommercial=false, tier=free) per P6's
// free-by-default canon; the artificial client-side "exit 2 without a
// license key" pre-check removed here (AMENDMENT 2026-09-03, P6-E3-W2-S1-T5
// FIX-PLUGINS) contradicted that manifest. A license key remains optional:
// when present it is still forwarded so any account-scoped ping_api
// features keep working; when absent the request is simply sent
// unauthenticated and ping_api's own response (mapMailError below) is
// surfaced normally, same as any other backend error.
//
// Inputs: environment variables (NSELF_PING_API_URL, license key env vars),
// the stored license key file.
//
// Outputs: a configured *mail.Client (a bearer license key attached when
// one is configured; empty otherwise — internal/mail.New already documents
// that an empty key is valid and lets the server decide).
//
// Constraints: no dependency on the core CLI's internal/* packages.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/nself-org/nself-mail/internal/licensekeys"
	"github.com/nself-org/nself-mail/internal/mail"
)

// resolveMailClient picks the ping_api base URL from env (NSELF_PING_API_URL,
// default https://ping.nself.org) and the first configured license key from
// internal/licensekeys.CollectLicenseKeys (which already reads
// NSELF_LICENSE_KEY, NSELF_PLUGIN_LICENSE_KEY, and NSELF_LICENSE_KEY_1..10),
// forwarding an empty key when none is configured — mail is a free plugin
// and no longer hard-blocks on a missing license (see package doc above).
func resolveMailClient() (*mail.Client, error) {
	pingURL := os.Getenv("NSELF_PING_API_URL")
	if pingURL == "" {
		pingURL = mail.DefaultPingURL
	}
	keys := licensekeys.CollectLicenseKeys()
	key := ""
	if len(keys) > 0 {
		key = keys[0]
	}
	return mail.New(pingURL, key), nil
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
