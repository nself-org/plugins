package mode

// Purpose: TestCoordinatorMode (P7-NODE-20, Epic D21, review F3): the stored mode, the personal-mode
// principal rule and the facts provider outside a serve.
// Inputs:  temporary stores, the roles service and a CA to create grants, tokens and user certificates.
// Outputs: pass or fail.
// Constraints: one subtest per acceptance line; no network; synthetic principal names only.

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type rig struct {
	t    *testing.T
	dir  string
	st   *store.Store
	svc  *roles.Service
	self roles.Actor
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := roles.New(st, &audit.Log{St: st})
	svc.Self = "owner-1"
	return &rig{t: t, dir: dir, st: st, svc: svc, self: svc.Operator("cli:owner-1")}
}

func (r *rig) modeAuditRows() int {
	r.t.Helper()
	rows, err := r.st.AuditEntries(context.Background(), 0, 0)
	if err != nil {
		r.t.Fatal(err)
	}
	n := 0
	for _, e := range rows {
		if e.Action == "coordinator.mode" {
			n++
		}
	}
	return n
}

func (r *rig) start(team bool) (Decision, error) {
	return Start(context.Background(), r.st, team, "operator:serve")
}

func TestCoordinatorMode(t *testing.T) {
	ctx := context.Background()
	t.Run("a fresh serve reports personal", func(t *testing.T) {
		r := newRig(t)
		d, err := r.start(false)
		if err != nil || d.Mode != Personal || d.Switched || r.modeAuditRows() != 0 {
			t.Fatalf("%+v %v audit=%d", d, err, r.modeAuditRows())
		}
	})
	t.Run("team switches with one audit row and a later start without the flag stays team", func(t *testing.T) {
		r := newRig(t)
		if _, err := r.start(false); err != nil {
			t.Fatal(err)
		}
		d, err := r.start(true)
		if err != nil || d.Mode != Team || !d.Switched || r.modeAuditRows() != 1 {
			t.Fatalf("%+v %v audit=%d", d, err, r.modeAuditRows())
		}
		for i := 0; i < 2; i++ {
			if d, err = r.start(false); err != nil || d.Mode != Team || d.Switched {
				t.Fatalf("restart without the flag: %+v %v", d, err)
			}
		}
		if r.modeAuditRows() != 1 {
			t.Fatalf("audit rows = %d, want 1", r.modeAuditRows())
		}
	})
	t.Run("a team dir never checks principals", func(t *testing.T) {
		r := newRig(t)
		if _, err := r.start(true); err != nil {
			t.Fatal(err)
		}
		if err := r.svc.Grant(ctx, r.self, roles.Request{Principal: "pal-2", Role: roles.Admin, Projects: []string{roles.AllProjects}}); err != nil {
			t.Fatal(err)
		}
		if d, err := r.start(false); err != nil || d.Mode != Team {
			t.Fatalf("%+v %v", d, err)
		}
	})
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, r *rig)
	}{
		{"an admin grant for a second identity", func(t *testing.T, r *rig) {
			if err := r.svc.Grant(ctx, r.self, roles.Request{Principal: "pal-2", Role: roles.Admin, Projects: []string{roles.AllProjects}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a viewer-level grant (any role counts)", func(t *testing.T, r *rig) {
			if err := r.svc.Grant(ctx, r.self, roles.Request{Principal: "pal-2", Role: roles.Member, Projects: []string{"proj-a"}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a token for a second identity", func(t *testing.T, r *rig) {
			if _, _, err := r.svc.IssueToken(ctx, r.self, roles.Request{Principal: "pal-2", Role: roles.Member, Projects: []string{"proj-a"}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"a user certificate for a second identity", func(t *testing.T, r *rig) {
			ca, err := pki.New(ctx, pki.Options{Store: r.st, DataDir: r.dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = ca.IssueUser(ctx, "pal-2", time.Hour); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run("personal dir with "+tc.name+" refuses with E664 personal_has_principals", func(t *testing.T) {
			r := newRig(t)
			if _, err := r.start(false); err != nil {
				t.Fatal(err)
			}
			tc.setup(t, r)
			_, err := r.start(false)
			e, ok := err.(*Error)
			if !ok || e.Code != "E664" || e.Reason != ReasonPersonalHasPrincipals || ReasonOf(err) != ReasonPersonalHasPrincipals {
				t.Fatalf("want E664 personal_has_principals, got %v", err)
			}
			for _, want := range []string{"--team", "revoke"} {
				if !strings.Contains(e.Message, want) {
					t.Fatalf("message must name %q: %s", want, e.Message)
				}
			}
			if r.st.CoordinatorMode(ctx) != Personal {
				t.Fatal("a refused start must not change the mode")
			}
			if d, err := r.start(true); err != nil || d.Mode != Team {
				t.Fatalf("--team is the way out: %+v %v", d, err)
			}
		})
	}
	t.Run("the operator's own --self member token does not count", func(t *testing.T) {
		r := newRig(t)
		if _, err := r.start(false); err != nil {
			t.Fatal(err)
		}
		for _, role := range []roles.Role{roles.Member, roles.Admin} {
			if _, _, err := r.svc.IssueToken(ctx, r.self, roles.Request{Principal: "owner-1", Role: role, Projects: []string{roles.AllProjects}, Self: true}); err != nil {
				t.Fatal(err)
			}
		}
		if err := r.svc.Grant(ctx, r.self, roles.Request{Principal: "owner-1", Role: roles.Admin, Projects: []string{roles.AllProjects}, Self: true}); err != nil {
			t.Fatal(err)
		}
		if d, err := r.start(false); err != nil || d.Mode != Personal {
			t.Fatalf("%+v %v", d, err)
		}
	})
	t.Run("an unreadable store refuses the start and reports team", func(t *testing.T) {
		r := newRig(t)
		_ = r.st.Close()
		d, err := r.start(false)
		if err == nil || d.Mode != Team || ReasonOf(err) != ReasonModeUnreadable {
			t.Fatalf("%+v %v", d, err)
		}
		if _, err = Start(ctx, nil, false, "operator:serve"); err == nil {
			t.Fatal("a nil store must refuse")
		}
	})
	t.Run("the provider returns Serving false when no serve runs", func(t *testing.T) {
		r := newRig(t)
		if f, err := Facts(ctx); err != nil || f.Serving || f.Mode != "" {
			t.Fatalf("no serve: %+v %v", f, err)
		}
		s := Begin(Options{Store: r.st})
		if _, err := r.start(false); err != nil {
			t.Fatal(err)
		}
		if f, _ := Facts(ctx); !f.Serving || f.Mode != Personal || !Serving() {
			t.Fatalf("serving: %+v", f)
		}
		if _, err := r.start(true); err != nil {
			t.Fatal(err)
		}
		if f, _ := Facts(ctx); f.Mode != Team {
			t.Fatalf("the mode is read live: %+v", f)
		}
		s.End()
		s.End()
		if f, err := Facts(ctx); err != nil || f.Serving || Serving() {
			t.Fatalf("after the serve ends: %+v %v", f, err)
		}
	})
	t.Run("a provider whose store cannot be read reports team", func(t *testing.T) {
		r := newRig(t)
		if _, err := r.start(false); err != nil {
			t.Fatal(err)
		}
		s := Begin(Options{Store: r.st})
		defer s.End()
		_ = r.st.Close()
		if f, _ := Facts(ctx); !f.Serving || f.Mode != Team {
			t.Fatalf("%+v", f)
		}
	})
}
