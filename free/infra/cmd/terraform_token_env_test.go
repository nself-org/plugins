package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// --token-env naming the .env-filled Terraform variable is refused, even with a value set,
// and a refused destroy sends nothing.
func TestServerRefusesTerraformVarAsTokenEnv(t *testing.T) {
	for _, c := range []*cobra.Command{serverDestroyCmd, serverListCmd, serverProvisionCmd, serverResizeCmd} {
		resetServerFlags()
		t.Setenv(TerraformTokenEnvVar, "from-project-env")
		_ = c.Flags().Set("token-env", TerraformTokenEnvVar)
		_, err := buildServerClient(c)
		if err == nil || !strings.Contains(err.Error(), "project .env") || !strings.Contains(err.Error(), "Terraform") {
			t.Fatalf("%s: want a refusal naming the reason, got %v", c.Name(), err)
		}
	}
	resetServerFlags()
	t.Setenv("HETZNER_NSELF_TOKEN", "")
	t.Setenv("HCLOUD_TOKEN", "")
	_ = serverDestroyCmd.Flags().Set("token-env", TerraformTokenEnvVar)
	_ = serverDestroyCmd.Flags().Set("id", "1")
	_ = serverDestroyCmd.Flags().Set("force-no-backup", "true")
	serverDestroyCmd.SetContext(context.Background())
	old := newServerClient
	newServerClient = buildServerClient
	defer func() { newServerClient = old }()
	if err := runServerDestroy(serverDestroyCmd, nil); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("destroy must refuse, got %v", err)
	}
}
