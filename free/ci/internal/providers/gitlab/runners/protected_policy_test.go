package runners_test

// Purpose: operator opt-in to protected-only runners without changing repository policy authority.
// Inputs: effective policies and a client whose authentication callback forbids requests.
// Outputs: explicit opt-in or E708 before API access.
// Constraints: no sockets and no credentials.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestProtectedRefsOnlyPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      bool
	}{{"default", "", false}, {"enabled", "true", true}, {"disabled", "false", false}} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{}
			if tc.raw != "" {
				values[runners.KeyProtectedRefsOnly] = tc.raw
			}
			eff, err := merge(t, scope(trust.SourceUser, values))
			if err != nil {
				t.Fatal(err)
			}
			got, err := runners.SettingsFrom(&eff)
			if err != nil || got.ProtectedRefsOnly != tc.want {
				t.Fatalf("settings = %+v, %v; want opt-in %v", got, err, tc.want)
			}
		})
	}
	for _, raw := range []string{`null`, `"true"`, `1`, `[]`, `{}`, `true false`} {
		if _, err := merge(t, scope(trust.SourceUser, map[string]string{runners.KeyProtectedRefsOnly: raw})); err == nil || !strings.Contains(err.Error(), "E672") {
			t.Errorf("value %s: want E672, got %v", raw, err)
		}
	}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob} {
		eff, err := merge(t, scope(kind, map[string]string{runners.KeyProtectedRefsOnly: `true`}))
		if err != nil {
			t.Fatal(err)
		}
		got, err := runners.SettingsFrom(&eff)
		if err != nil || got.ProtectedRefsOnly || len(eff.Ignored()) != 1 {
			t.Fatalf("%s opted in: %+v, %v, ignored %v", kind, got, err, eff.Ignored())
		}
	}
}

func TestMintRequiresProtectedRefsOnlyOptIn(t *testing.T) {
	for _, name := range []string{"default", "explicit false"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			cfg := runners.Config{Client: &rest.Client{BaseURL: "https://gitlab.example.com/api/v4", Provider: "gitlab", Auth: func(context.Context) (string, error) {
				calls++
				return "", fmt.Errorf("API access forbidden")
			}}}
			if name == "explicit false" {
				cfg.Settings = runners.Settings{ProtectedRefsOnly: false, Tags: []string{"nself"}}
			}
			reg, err := runners.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			minted, err := reg.MintRunner(ctx, runnerReq(), runners.MintOptions{})
			if code(err) != "E708" || minted.Credential.Value != "" || minted.RunnerID != "" {
				t.Fatalf("MintRunner = %v, %v", minted, err)
			}
			cred, err := reg.Mint(ctx, runnerReq())
			if code(err) != "E708" || cred.Value != "" {
				t.Fatalf("Mint = %v, %v", cred, err)
			}
			m := &runners.Minter{Registrar: func(context.Context) (*runners.Registrar, error) { return reg, nil }}
			got, err := m.Mint(ctx, runnerjob.Request{Repo: testProject, Demand: "1", AttemptID: "a1"})
			if code(err) != "E708" || got.Credential != "" || got.Handle != "" || calls != 0 {
				t.Fatalf("Minter = %+v, %v; API attempts %d", got, err, calls)
			}
		})
	}
}

func TestDefaultMinterRequiresProtectedRefsOnlyOptIn(t *testing.T) {
	runners.UseRegistrar(nil)
	runners.UsePolicy(nil)
	got, err := (&runners.Minter{}).Mint(context.Background(), runnerjob.Request{Repo: testProject, Demand: "1", AttemptID: "a1"})
	if code(err) != "E708" || got.Credential != "" || got.Handle != "" {
		t.Fatalf("default Minter = %+v, %v; want E708 without a credential", got, err)
	}
}
