// gateway_env.go — gateway base URL resolution for nself-ci.
//
// Purpose: One resolver for the gateway routing check target, shared by the
//
//	"run" pipeline command and the single-repo gate.
//
// Inputs:  env (--env flag), gateway (--gateway flag), envVar (NSELF_CI_GATEWAY value).
// Outputs: gateway base URL ("" = skip the routing check), or an error for an unknown --env.
// Precedence: explicit gateway > --env preset > NSELF_CI_GATEWAY.
//
// SPORT: PLUGINS-CI-005
package main

import "fmt"

// gatewayBaseForEnv maps --env values to gateway base URLs.
// Only the local stack is a preset; remote targets use --gateway or NSELF_CI_GATEWAY.
// NEVER add production IP here (5.75.235.42 is deny-listed per destructive-deny-list.md).
var gatewayBaseForEnv = map[string]string{
	"local": "http://127.0.0.1:3761",
}

// resolveGatewayBase returns the gateway base URL for the routing check.
// An explicit gateway wins over env; envVar applies only when both are empty.
// An unknown non-empty env is an error that names the replacement options.
func resolveGatewayBase(env, gateway, envVar string) (string, error) {
	base := gateway
	if base == "" && env != "" {
		b, ok := gatewayBaseForEnv[env]
		if !ok {
			return "", fmt.Errorf("unknown --env %q (valid: local). The staging preset was removed: its host is no longer operated by nSelf. Use --gateway <url> or NSELF_CI_GATEWAY", env)
		}
		base = b
	}
	if base == "" {
		base = envVar
	}
	return base, nil
}
