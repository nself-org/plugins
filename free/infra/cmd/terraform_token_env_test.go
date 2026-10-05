package main

import (
	"os"
	"testing"
)

func TestApplyTerraformTokenEnv(t *testing.T) {
	t.Setenv("HCLOUD_TOKEN", "")
	t.Setenv(TerraformTokenEnvVar, "tf-token")
	applyTerraformTokenEnv()
	if got := os.Getenv("HCLOUD_TOKEN"); got != "tf-token" {
		t.Fatalf("HCLOUD_TOKEN = %q, want tf-token", got)
	}
	t.Setenv("HCLOUD_TOKEN", "operator")
	applyTerraformTokenEnv()
	if got := os.Getenv("HCLOUD_TOKEN"); got != "operator" {
		t.Fatalf("an operator HCLOUD_TOKEN must win, got %q", got)
	}
}

// The server commands must not read the Terraform-only variable.
func TestServerTokenIgnoresTerraformVar(t *testing.T) {
	resetServerFlags()
	t.Setenv("HETZNER_NSELF_TOKEN", "")
	t.Setenv("HCLOUD_TOKEN", "")
	t.Setenv(TerraformTokenEnvVar, "tf-token")
	if _, err := buildServerClient(serverDestroyCmd); err == nil {
		t.Fatal("server commands resolved a token from the Terraform-only variable")
	}
}
