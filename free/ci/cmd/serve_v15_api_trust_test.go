package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestDispatchAPITrustLiveAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, label, project, want string
		role                       roles.Role
		reachable, payload, fail   bool
		wantErr                    bool
	}{
		{"owner", "user:bob", "o/r", "owner", roles.Admin, true, false, false, false},
		{"member", "user:bob", "o/r", "internal", roles.Member, true, false, false, false},
		{"release", "user:bob", "o/r", "internal", roles.Release, true, false, false, false},
		{"owner arbitrary commit", "user:bob", "o/r", "untrusted", roles.Admin, false, false, false, false},
		{"member arbitrary commit", "user:bob", "o/r", "untrusted", roles.Member, false, false, false, false},
		{"payload ancestry", "user:bob", "o/r", "untrusted", roles.Admin, true, true, false, false},
		{"forge unavailable", "user:bob", "o/r", "untrusted", roles.Admin, true, false, true, false},
		{"grant removed", "user:bob", "o/r", "untrusted", "", true, false, false, true},
		{"wrong project", "user:bob", "other/repo", "untrusted", roles.Admin, true, false, false, true},
		{"no submitter", "", "o/r", "untrusted", roles.Admin, true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, home := context.Background(), t.TempDir()
			st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.role != "" {
				svc := roles.New(st, &audit.Log{St: st})
				if err := svc.Grant(ctx, svc.Operator("local"), roles.Request{Principal: "bob", Role: tc.role, Projects: []string{"o/r"}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			sha := strings.Repeat("a", 40)
			facts, err := dispatchTrustFacts(store.IntakeRow{Source: "manual", Project: tc.project, Revision: sha,
				Facts: []byte(`{"trigger":"api","submitter":"user:bob"}`)}, pipelineFacts{Trigger: "api", Submitter: tc.label})
			if err != nil {
				t.Fatal(err)
			}
			h := forgeDispatchTrust{facts: facts, home: home, submitter: tc.label, hook: cirun.SourceTrust{
				Getenv: func(string) string { return "" },
				Refetch: func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
					if tc.fail {
						return f, errors.New("forge offline")
					}
					if f.Source != "github" || f.Repo != tc.project || f.Revision != sha || f.Actor.Value != nil || f.ActorPermission.Value != nil || f.SHAReachable.Value != nil {
						t.Fatalf("query borrowed authority or changed intake: %+v", f)
					}
					f.SHAReachable = apiDispatchFact(tc.reachable)
					if tc.payload {
						f.SHAReachable.Source = "payload"
					}
					return f, nil
				}}}
			got, err := h.Classify(ctx, cirun.TrustRequest{Root: t.TempDir(), Revision: sha})
			if string(got) != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("class=%s err=%v; want %s error=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
