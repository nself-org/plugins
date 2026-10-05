package main

// Purpose: give the Terraform commands (plan, apply, destroy) a project-.env
// token source that the Hetzner server commands can never see (P7-CANON-12).
// Inputs: the process environment.
// Outputs: HCLOUD_TOKEN set from TerraformTokenEnvVar when HCLOUD_TOKEN is unset.
// Constraints: the 1.4.x proxy fills every manifest `envVars` name from the
// project .env cascade. `nself infra server` must read only core's token
// sources (--token, --token-env, process env HETZNER_NSELF_TOKEN / HCLOUD_TOKEN),
// so the manifest declares ONLY TerraformTokenEnvVar here, never those two names.
// A token in the project .env therefore reaches Terraform (under this name) and
// never a server provision/resize/destroy.

import "os"

// TerraformTokenEnvVar is the manifest-declared, Terraform-only token variable.
const TerraformTokenEnvVar = "NSELF_INFRA_TERRAFORM_HCLOUD_TOKEN"

// applyTerraformTokenEnv exports HCLOUD_TOKEN for the Terraform provider from
// TerraformTokenEnvVar when the operator has not set HCLOUD_TOKEN themselves.
func applyTerraformTokenEnv() {
	if tok := os.Getenv(TerraformTokenEnvVar); tok != "" && os.Getenv("HCLOUD_TOKEN") == "" {
		os.Setenv("HCLOUD_TOKEN", tok)
	}
}
