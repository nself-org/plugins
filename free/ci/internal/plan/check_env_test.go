package plan

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/config"
)

func TestBuiltinCheckRegistrySettings(t *testing.T) {
	loaded, err := config.Parse("ci.yaml", []byte(`version: 1
checks:
  node.lint:
    secrets: [project/NPM_TOKEN]
    env: {NSELF_CI_LOCAL_REGISTRY_INFRA: "1"}
`))
	if err != nil {
		t.Fatal(err)
	}
	p := mustBuild(t, Input{Config: loaded.Config, World: linuxWorld("npm"), Checks: []Check{
		{ID: "node.lint", Kind: "lint", Tool: "npm", Argv: []string{"npm", "run", "lint"}, Workdir: ".", Required: true, Path: "fast"},
	}})
	if len(p.Jobs) != 1 {
		t.Fatalf("jobs: %+v", p.Jobs)
	}
	j := p.Jobs[0].Job
	if len(j.Secrets) != 1 || j.Secrets[0] != "project/NPM_TOKEN" || j.Env["NSELF_CI_LOCAL_REGISTRY_INFRA"] != "1" {
		t.Fatalf("job lost registry settings: %+v", j)
	}
	loaded.Config.Checks["node.lint"].Secrets[0] = "project/OTHER"
	loaded.Config.Checks["node.lint"].Env["NSELF_CI_LOCAL_REGISTRY_INFRA"] = "0"
	if j.Secrets[0] != "project/NPM_TOKEN" || j.Env["NSELF_CI_LOCAL_REGISTRY_INFRA"] != "1" {
		t.Fatal("plan aliases authored check settings")
	}
}
