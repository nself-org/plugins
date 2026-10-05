package main

// Purpose: shared flag handling for the `nself infra server` subcommands —
// turning --token/--token-env into a server.Client. Split out so the
// provision/list/resize/destroy handlers stay pure cobra wiring, mirroring
// access_transport.go's newAccessTransport pattern.
// Inputs: a *cobra.Command carrying --token/--token-env flags.
// Outputs: a server.Client (always a real Hetzner client in the live CLI —
// newServerClient is swapped for a fake in server_test.go so command tests
// never reach the network) or a resolution error.
// Constraints: never logs or echoes the resolved token.

import (
	"fmt"

	"github.com/nself-org/nself-infra/internal/server"

	"github.com/spf13/cobra"
)

// newServerClient is a package-level indirection so server_test.go can
// substitute a fake Client without any handler knowing the difference. The
// real CLI never reassigns it — every live invocation resolves through
// buildServerClient to a real Hetzner-backed client.
var newServerClient = buildServerClient

// buildServerClient resolves --token/--token-env into a token and builds a
// live Hetzner Client from it. It never makes a network call itself; that
// only happens when a handler invokes an operation on the returned Client.
func buildServerClient(cmd *cobra.Command) (server.Client, error) {
	explicit, _ := cmd.Flags().GetString("token")
	envVar, _ := cmd.Flags().GetString("token-env")

	// The proxy fills TerraformTokenEnvVar from the project .env cascade; a server
	// command must never take a cloud token from a project file (P7-CANON-12).
	if envVar == TerraformTokenEnvVar {
		return nil, fmt.Errorf("nself infra server: --token-env %s is refused: that variable is filled from the project .env file and is for the Terraform commands only; export the token in your shell or pass --token", TerraformTokenEnvVar)
	}

	token, err := server.ResolveToken(explicit, envVar)
	if err != nil {
		return nil, fmt.Errorf("nself infra server: %w", err)
	}
	return server.NewHetznerClient(token), nil
}
