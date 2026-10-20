//go:build security

package redteam

// Purpose: red team group C, machine-invoked trust cap and the `--trust owner` override (TM-37, TM-44).
// The attacker is a machine-invoked caller that asks for a class above its cap, spells the class
// differently, sets the invoker variable to whitespace, or makes the override audit row unwritable.
// Inputs:  the production run.SourceTrust classifier with an injected environment, git runner and store.
// Outputs: pass (no class above the cap, an override only with its audit row) or fail.
// Constraints: temp dirs only; the git runner fails every call, so the classified class is untrusted.

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestC37C44TrustOverrideAndCap(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	svc := roles.New(st, &audit.Log{St: st})
	w := &policy.Writer{Store: st, Log: &audit.Log{St: st}}
	if _, err := w.Apply(ctx, svc.Operator("cli:alice"), policy.Change{Op: policy.OpSet, Scope: "user", Key: "trust.machine_invoked_cap", Value: json.RawMessage(`"collaborator"`)}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	open := func() (*store.Store, error) { return store.Open(path, store.Options{}) }
	capture := func(context.Context, ...string) (string, int, error) { return "", 128, nil }
	rev := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	classifier := func(invoker string, openStore func() (*store.Store, error)) run.SourceTrust {
		return run.SourceTrust{Capture: capture, OpenStore: openStore, Getenv: func(k string) string {
			if k == "NSELF_INVOKED_BY" {
				return invoker
			}
			return ""
		}}
	}
	count := func() int {
		s, _ := open()
		defer s.Close()
		rows, _ := s.AuditEntries(ctx, 0, 1000)
		n := 0
		for _, r := range rows {
			if r.Action == "trust.override" {
				n++
			}
		}
		return n
	}
	for _, inv := range []string{"mcp", " ", "\t", "0", "false"} {
		for _, req := range []string{"owner", "internal"} {
			res, err := classifier(inv, open).Decide(ctx, run.TrustRequest{Root: t.TempDir(), Revision: rev, Requested: req})
			if err == nil || res.Class == "owner" || res.Class == "internal" {
				t.Errorf("invoker %q asked --trust %s: class %q err %v", inv, req, res.Class, err)
			}
		}
	}
	if n := count(); n != 0 {
		t.Errorf("a refused machine override wrote %d override rows", n)
	}
	for _, req := range []string{"OWNER", "Owner", " owner", "owner ", "owner\n", "owker", "owner\x00", "0wner"} {
		res, err := classifier("", open).Decide(ctx, run.TrustRequest{Root: t.TempDir(), Revision: rev, Requested: req})
		if err == nil && res.Class == "owner" {
			t.Errorf("--trust %q produced the owner class", req)
		}
	}
	if n := count(); n != 0 {
		t.Errorf("a respelled class wrote %d override rows", n)
	}
	broken := func() (*store.Store, error) { return nil, errors.New("store unavailable") }
	if res, err := classifier("", broken).Decide(ctx, run.TrustRequest{Root: t.TempDir(), Revision: rev, Requested: "owner"}); err == nil || res.Class == "owner" {
		t.Errorf("--trust owner without an audit row: class %q err %v", res.Class, err)
	}
	res, err := classifier("", open).Decide(ctx, run.TrustRequest{Root: t.TempDir(), Revision: rev, Requested: "owner"})
	if err != nil || res.Class != "owner" || count() != 1 {
		t.Errorf("control: the audited operator override: class %q err %v rows %d", res.Class, err, count())
	}
}
