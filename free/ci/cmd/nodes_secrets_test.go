package main

// Purpose: `nodes secrets grant|revoke` refuses an ineligible node before any cli call or audit row,
// grants release only to a dedicated-trusted node, and calls `config secrets recipients` once with the
// node's recipient and writes one audit row on success (P7-TRUST-02).

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const testRecipient = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

type recordingRunner struct {
	mu    sync.Mutex
	calls []secrets.Call
	out   secrets.Output
}

func (r *recordingRunner) Run(_ context.Context, c secrets.Call) (secrets.Output, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
	return r.out, nil
}

type secretsEnv struct {
	path string
	run  *recordingRunner
	out  *bytes.Buffer
	errs *bytes.Buffer
	deps nodesSecretsDeps
}

// newSecretsEnv registers one node (uid separation decides secrets_eligible) and pins its recipient.
func newSecretsEnv(t *testing.T, uidSeparation, pinRecipient bool) (*secretsEnv, string) {
	t.Helper()
	b, err := os.ReadFile("../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.Separation.UIDSeparation.Value = &uidSeparation
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	ctx := context.Background()
	if _, err = reg.Register(ctx, c, "operator"); err != nil {
		t.Fatal(err)
	}
	if pinRecipient {
		if err = reg.CheckAgeRecipient(ctx, c.Identity.ID, testRecipient, "operator"); err != nil {
			t.Fatal(err)
		}
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	e := &secretsEnv{path: path, run: &recordingRunner{}, out: &bytes.Buffer{}, errs: &bytes.Buffer{}}
	e.deps = nodesSecretsDeps{
		OpenStore: func() (*store.Store, error) { return store.Open(path, store.Options{}) },
		Runner:    e.run, Actor: "tester", Getwd: func() (string, error) { return "/work/proj", nil },
		ProjectID: func(_ context.Context, root string) string { return root },
		Stdout:    e.out, Stderr: e.errs,
	}
	return e, c.Identity.ID
}

// auditRows counts rows of one action by reading the store files (the store owns the SQL).
func (e *secretsEnv) auditRows(t *testing.T, action string) int {
	t.Helper()
	dir := filepath.Dir(e.path)
	ents, _ := os.ReadDir(dir)
	n := 0
	for _, f := range ents {
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			t.Fatal(err)
		}
		n += bytes.Count(b, []byte(action))
	}
	return n
}

func TestNodeNotEligibleGrant(t *testing.T) {
	e, id := newSecretsEnv(t, false, true)
	code := runNodesSecrets("grant", []string{id, "--class", "project"}, e.deps)
	if code != 1 || !strings.Contains(e.errs.String(), "E677") || !strings.Contains(e.errs.String(), "secrets.node_not_eligible") || !strings.Contains(e.errs.String(), id) {
		t.Fatalf("code=%d err=%q", code, e.errs.String())
	}
	if len(e.run.calls) != 0 {
		t.Fatalf("an ineligible node reached the cli: %v", e.run.calls)
	}
	if n := e.auditRows(t, "node.secrets."); n != 0 {
		t.Fatalf("%d audit rows for a refused grant", n)
	}
}

func TestGrantProjectCallsCliAndAudits(t *testing.T) {
	e, id := newSecretsEnv(t, true, true)
	if code := runNodesSecrets("grant", []string{id, "--class", "project"}, e.deps); code != 0 {
		t.Fatalf("code=%d err=%q", code, e.errs.String())
	}
	if len(e.run.calls) != 1 {
		t.Fatalf("calls %v", e.run.calls)
	}
	argv := strings.Join(e.run.calls[0].Argv, " ")
	want := "nself config secrets recipients add --file ci --project /work/proj " + testRecipient
	if argv != want {
		t.Fatalf("argv %q want %q", argv, want)
	}
	if n := e.auditRows(t, "node.secrets.grant"); n != 1 {
		t.Fatalf("%d audit rows, want 1", n)
	}
}

func TestGrantReleaseNeedsDedicatedNode(t *testing.T) {
	e, id := newSecretsEnv(t, true, true)
	code := runNodesSecrets("grant", []string{id, "--class", "release"}, e.deps)
	if code != 1 || !strings.Contains(e.errs.String(), "E677") || len(e.run.calls) != 0 || e.auditRows(t, "node.secrets.") != 0 {
		t.Fatalf("code=%d err=%q calls=%v", code, e.errs.String(), e.run.calls)
	}
}

func TestGrantNeedsRecipient(t *testing.T) {
	e, id := newSecretsEnv(t, true, false)
	code := runNodesSecrets("grant", []string{id, "--class", "project"}, e.deps)
	if code != 1 || !strings.Contains(e.errs.String(), "secrets.node_no_recipient") || len(e.run.calls) != 0 {
		t.Fatalf("code=%d err=%q", code, e.errs.String())
	}
}

func TestRevokeWorksForIneligibleNode(t *testing.T) {
	e, id := newSecretsEnv(t, false, true)
	if code := runNodesSecrets("revoke", []string{id, "--class", "environment/staging"}, e.deps); code != 0 {
		t.Fatalf("code=%d err=%q", code, e.errs.String())
	}
	if len(e.run.calls) != 1 || !strings.Contains(strings.Join(e.run.calls[0].Argv, " "), "recipients remove --file ci-staging") {
		t.Fatalf("calls %v", e.run.calls)
	}
	if !strings.Contains(e.out.String(), "rotate") || e.auditRows(t, "node.secrets.revoke") != 1 {
		t.Fatalf("out=%q", e.out.String())
	}
}

func TestGrantCliRefusalWritesNoAudit(t *testing.T) {
	e, id := newSecretsEnv(t, true, true)
	e.run.out = secrets.Output{ExitCode: 1, Bytes: []byte(`{"error":{"code":"E544","message":"x"}}`)}
	code := runNodesSecrets("grant", []string{id, "--class", "project"}, e.deps)
	if code != 1 || !strings.Contains(e.errs.String(), "E544") || e.auditRows(t, "node.secrets.") != 0 {
		t.Fatalf("code=%d err=%q", code, e.errs.String())
	}
}

func TestNodesSecretsUsageAndRegistration(t *testing.T) {
	for _, k := range []string{"nodes secrets grant", "nodes secrets revoke"} {
		if _, ok := v15Subcommands[k]; !ok {
			t.Errorf("%q is not registered", k)
		}
	}
	e, id := newSecretsEnv(t, true, true)
	for _, args := range [][]string{{}, {id}, {id, "--class", "bogus"}, {id, "--class", "environment"}, {"nosuchnode", "--class", "project"}} {
		if code := runNodesSecrets("grant", args, e.deps); code != 1 {
			t.Fatalf("%v: code %d", args, code)
		}
	}
	if len(e.run.calls) != 0 {
		t.Fatal("a bad invocation reached the cli")
	}
}
