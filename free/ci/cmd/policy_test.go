package main

// Purpose: tests of `nself ci policy show|set|unset|pin` (P7-TRUST-26): every write is admin-gated and
// writes one audit row in its own transaction, a member is refused with nothing written, a pin must be
// reachable from the fetched default branch, `show --json` carries the contract:ci.policy data, and a
// machine-invoked caller cannot loosen a trust key.
// Inputs:  a real SQLite store, a bare origin and a clone in t.TempDir(), the commands' own run function.
// Outputs: pass or fail; no network. Fake values only.
// Constraints: each test named by the Ticket's verify line asserts PASS and never skips; a missing git is a
//          t.Fatal.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const fakeSecretKey = "test.fake_token"

func init() {
	err := policy.RegisterKey(policy.KeySpec{Key: fakeSecretKey, Owner: "test", Kind: policy.TightenAny, Secret: true,
		Decode: func(raw json.RawMessage) (any, error) { var s string; return s, json.Unmarshal(raw, &s) }})
	if err != nil {
		panic(err)
	}
}

func policyGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

type policyRig struct {
	*adminRig
	work    string
	token   string // run as this token's actor instead of the operator
	machine bool
}

func newPolicyRig(t *testing.T, branchPolicy string) *policyRig {
	t.Helper()
	root := t.TempDir()
	origin, work := filepath.Join(root, "origin.git"), filepath.Join(root, "work")
	policyGit(t, root, "init", "-q", "--bare", "-b", "main", origin)
	policyGit(t, root, "init", "-q", "-b", "main", work)
	policyGit(t, work, "remote", "add", "origin", origin)
	if err := os.MkdirAll(filepath.Join(work, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".nself/ci.yaml"), []byte("version: 1\n"+branchPolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	policyGit(t, work, "add", ".")
	policyGit(t, work, "commit", "-q", "-m", "base")
	policyGit(t, work, "push", "-q", "origin", "main")
	policyGit(t, work, "remote", "set-head", "origin", "main")
	return &policyRig{adminRig: newAdminRig(t), work: work}
}

func (r *policyRig) env() policyEnv {
	ae := r.adminRig.env()
	open := ae.Open
	ae.Open = func(ctx context.Context) (*roles.Service, roles.Actor, func(), error) {
		svc, op, closeFn, err := open(ctx)
		if err != nil || r.token == "" {
			return svc, op, closeFn, err
		}
		actor, err := svc.ActorFromToken(ctx, r.token)
		return svc, actor, closeFn, err
	}
	return policyEnv{adminEnv: ae, Run: policy.Runner(report.Capture), Root: func() (string, error) { return r.work, nil }, Machine: r.machine}
}

func (r *policyRig) do(op string, args ...string) int { return runPolicy(op, args, r.env()) }

// memberToken issues a member token for bob on every project.
func (r *policyRig) memberToken() string {
	r.t.Helper()
	ctx := context.Background()
	svc, op, closeFn, err := r.adminRig.env().Open(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	defer closeFn()
	tok, _, err := svc.IssueToken(ctx, op, roles.Request{Principal: "bob", Role: roles.Member, Projects: []string{"*"}, TTL: time.Hour})
	if err != nil {
		r.t.Fatal(err)
	}
	return tok
}

func (r *policyRig) store() *store.Store {
	r.t.Helper()
	st, err := store.Open(r.path, store.Options{Clock: r.now})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = st.Close() })
	return st
}

func (r *policyRig) project() string {
	return policy.ProjectIDGit(context.Background(), policy.Runner(report.Capture), r.work)
}

// stored is the stored JSON of key in scope for the rig's subjects ("" when there is no row).
func (r *policyRig) stored(scope, key string) string {
	r.t.Helper()
	subject := map[string]string{"user": policy.LocalSubject, "project": r.project()}[scope]
	rows, err := r.store().PolicyRows(context.Background(), scope, subject)
	if err != nil {
		r.t.Fatal(err)
	}
	for _, row := range rows {
		if row.Key == key {
			return string(row.Value)
		}
	}
	return ""
}

func (r *policyRig) auditCount(action string) int {
	r.t.Helper()
	rows, err := r.store().AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		r.t.Fatal(err)
	}
	n := 0
	for _, e := range rows {
		if e.Action == action {
			n++
		}
	}
	return n
}

// errCode is the error envelope's code and reason of the last --json command.
func (r *policyRig) errCode() string {
	r.t.Helper()
	var env struct {
		Error struct{ Code, Message string } `json:"error"`
	}
	if err := json.Unmarshal(r.out.Bytes(), &env); err != nil {
		r.t.Fatalf("not one JSON envelope: %v\n%s", err, r.out.String())
	}
	return env.Error.Code + " " + env.Error.Message
}

func TestPolicySetAudit(t *testing.T) {
	r := newPolicyRig(t, "")
	if code := r.do("set", "--scope", "user", "--json", "trust.network", "restricted"); code != 0 {
		t.Fatalf("set exited %d: %s %s", code, r.out.String(), r.errb.String())
	}
	if got := r.stored("user", "trust.network"); got != `"restricted"` {
		t.Fatalf("stored value %q", got)
	}
	if n := r.auditCount("policy.set"); n != 1 {
		t.Fatalf("one audit row per set, got %d", n)
	}
	if code := r.do("set", "--scope", "user", "trust.network", "none"); code != 0 || r.auditCount("policy.set") != 2 {
		t.Fatalf("a second set tightens and writes its own row: exit %d, rows %d", code, r.auditCount("policy.set"))
	}
	if code := r.do("unset", "--scope", "user", "trust.network"); code != 0 || r.stored("user", "trust.network") != "" || r.auditCount("policy.unset") != 1 {
		t.Fatalf("unset removes the row and audits once: exit %d", code)
	}
	// Refusals write nothing: an unknown key, a value outside the order, a row that loosens a parent.
	before := r.auditCount("policy.set")
	for _, bad := range [][]string{{"--scope", "user", "no.such_key", "x"}, {"--scope", "user", "trust.network", "bogus"}, {"--scope", "mars", "trust.network", "none"}} {
		if code := r.do("set", append([]string{"--json"}, bad...)...); code != 1 {
			t.Fatalf("set %v exited %d, want 1: %s", bad, code, r.out.String())
		}
	}
	if code := r.do("set", "--scope", "user", "trust.isolation_min", "process"); code != 1 {
		t.Fatalf("a row looser than the untrusted default must be refused, exit %d", code)
	}
	if r.stored("user", "trust.isolation_min") != "" || r.auditCount("policy.set") != before {
		t.Fatalf("a refused write left a row or an audit row")
	}
	if code := r.do("set", "--scope", "user", "trust.network", "none"); code != 0 {
		t.Fatal("setup")
	}
	if code := r.do("set", "--json", "--scope", "project", "trust.network", "internet"); code != 1 || !strings.HasPrefix(r.errCode(), "E670") {
		t.Fatalf("a project row looser than the user row is E670, got exit %d %s", code, r.out.String())
	}
}

// failingScanner refuses any audit event that holds the fake value.
type failingScanner struct{}

func (failingScanner) Scan(blob []byte) []redact.Match {
	if i := bytes.Index(blob, []byte("fake-registered-value")); i >= 0 {
		return []redact.Match{{Offset: i, Len: 21}}
	}
	return nil
}

func TestPolicyWriteAtomicWithAudit(t *testing.T) {
	r := newPolicyRig(t, "")
	audit.SetScanner(failingScanner{})
	t.Cleanup(func() { audit.SetScanner(nil) })
	if code := r.do("set", "--scope", "user", "--json", "trust.providers", `["fake-registered-value"]`); code == 0 {
		t.Fatal("a write whose audit row is refused must fail")
	}
	if got := r.stored("user", "trust.providers"); got != "" {
		t.Fatalf("the row must roll back with its audit row, found %s", got)
	}
}

func TestPolicySetMemberRefused(t *testing.T) {
	r := newPolicyRig(t, "")
	r.token = r.memberToken()
	for _, args := range [][]string{{"set", "--scope", "user", "--json", "trust.network", "none"}, {"unset", "--scope", "user", "--json", "trust.network"}, {"pin", "--json", "0000000000000000000000000000000000000001"}} {
		if code := r.do(args[0], args[1:]...); code != 1 || !strings.HasPrefix(r.errCode(), "E687") {
			t.Fatalf("%v: a TEAM member gets E687, exit %d: %s", args, code, r.out.String())
		}
	}
	if r.stored("user", "trust.network") != "" || r.auditCount("policy.set")+r.auditCount("policy.unset")+r.auditCount("policy.pin") != 0 {
		t.Fatalf("nothing is written for a member")
	}
}

func TestPolicyPinReachable(t *testing.T) {
	r := newPolicyRig(t, "")
	base := policyGit(t, r.work, "rev-parse", "HEAD")
	policyGit(t, r.work, "commit", "-q", "--allow-empty", "-m", "second")
	policyGit(t, r.work, "push", "-q", "origin", "main")
	tip := policyGit(t, r.work, "rev-parse", "HEAD")
	policyGit(t, r.work, "checkout", "-q", "-b", "side", base)
	policyGit(t, r.work, "commit", "-q", "--allow-empty", "-m", "side")
	policyGit(t, r.work, "push", "-q", "origin", "side")
	side := policyGit(t, r.work, "rev-parse", "HEAD")
	policyGit(t, r.work, "checkout", "-q", "main")

	pinned := func() string {
		pin, ok, err := r.store().PolicyPinFor(context.Background(), r.project())
		if err != nil || !ok {
			return ""
		}
		return pin.SHA
	}
	for _, bad := range []string{side, base[:12], "refs/heads/main", strings.Repeat("a", 40)} {
		if code := r.do("pin", "--json", bad); code != 1 || !strings.HasPrefix(r.errCode(), "E672") {
			t.Fatalf("pin %s must be E672, got exit %d: %s", bad, code, r.out.String())
		}
	}
	if pinned() != "" || r.auditCount("policy.pin") != 0 {
		t.Fatal("a refused pin wrote something")
	}
	if code := r.do("pin", "--json", base); code != 0 || pinned() != base || r.auditCount("policy.pin") != 1 {
		t.Fatalf("pin of a reachable commit: exit %d pinned %q: %s", code, pinned(), r.out.String())
	}
	if code := r.do("pin", tip); code != 0 || pinned() != tip {
		t.Fatalf("pin of the tip: exit %d", code)
	}
	if code := r.do("pin", "--clear"); code != 0 || pinned() != "" || r.auditCount("policy.clear") != 1 {
		t.Fatalf("--clear returns to the tip with one audit row: exit %d", code)
	}
}

func TestPolicyShowJSON(t *testing.T) {
	r := newPolicyRig(t, "policy:\n  trust:\n    privacy_zone: private-infrastructure\n  economics:\n    paid_enabled: true\n")
	policyGit(t, r.work, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(r.work, ".nself/ci.yaml"), []byte("version: 1\npolicy:\n  trust:\n    privacy_zone: hosted-allowed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	show := func() map[string]any {
		t.Helper()
		if code := r.do("show", "--json"); code != 0 {
			t.Fatalf("show exited %d: %s %s", code, r.out.String(), r.errb.String())
		}
		return r.data()
	}
	d := show()
	for _, k := range []string{"effective", "sources", "pinned_commit", "fetch", "digest", "ignored"} {
		if _, ok := d[k]; !ok {
			t.Fatalf("show --json data lacks %s: %v", k, d)
		}
	}
	eff := d["effective"].(map[string]any)
	if eff["trust.privacy_zone"] != "private-infrastructure" || d["fetch"] != "fresh" || d["pinned_commit"] == "" {
		t.Fatalf("the protected branch wins over the worktree: %v", d)
	}
	ignored := map[string]string{}
	for _, ig := range d["ignored"].([]any) {
		m := ig.(map[string]any)
		ignored[m["key"].(string)] = m["code"].(string) + "/" + m["source"].(string)
	}
	if ignored["trust.privacy_zone"] != "E671/revision" || ignored[schedpolicy.KeyPaidEnabled] != "E671/protected-branch" {
		t.Fatalf("the widening revision and the branch's Spend key are listed as ignored: %v", ignored)
	}
	if eff[schedpolicy.KeyPaidEnabled] != false {
		t.Fatalf("a Spend key in the branch must not apply: %v", eff[schedpolicy.KeyPaidEnabled])
	}
	// A Spend key set at project scope applies.
	if code := r.do("set", "--scope", "project", schedpolicy.KeyPaidEnabled, "true"); code != 0 {
		t.Fatalf("set spend key exited %d: %s", code, r.errb.String())
	}
	d = show()
	if d["effective"].(map[string]any)[schedpolicy.KeyPaidEnabled] != true || d["sources"].(map[string]any)[schedpolicy.KeyPaidEnabled] != "project" {
		t.Fatalf("a project-scope Spend key applies: %v", d)
	}
	// A Secret key prints masked everywhere and its value never reaches an audit row.
	const secret = "fake-secret-value-0001"
	if code := r.do("set", "--scope", "user", "--json", fakeSecretKey, secret); code != 0 || strings.Contains(r.out.String(), secret) {
		t.Fatalf("set of a Secret key: exit %d, output %s", code, r.out.String())
	}
	for _, args := range [][]string{{"--json"}, {}, {"--effective"}} {
		if r.do("show", args...); strings.Contains(r.out.String()+r.errb.String(), secret) {
			t.Fatalf("show %v printed a Secret value", args)
		}
	}
	if show()["effective"].(map[string]any)[fakeSecretKey] != policy.Masked {
		t.Fatal("a Secret key must print as [REDACTED]")
	}
	entries, err := r.store().AuditEntries(context.Background(), 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Detail+e.Target+e.Action, secret) {
			t.Fatalf("a Secret value reached the audit log: %+v", e.AuditRow)
		}
	}
}

func TestPolicyMachineInvokedCap(t *testing.T) {
	r := newPolicyRig(t, "")
	capKey := "trust.machine_invoked_cap"
	refused := func(what string, code int) {
		t.Helper()
		if code != 1 || !strings.Contains(r.errCode(), "E687") || !strings.Contains(r.errCode(), policy.ReasonMachineInvoked) {
			t.Fatalf("%s must fail with E687 machine_invoked, got exit %d: %s", what, code, r.out.String())
		}
	}
	r.machine = true
	// Lowering a trust key works for a machine-invoked caller.
	if code := r.do("set", "--scope", "user", capKey, "collaborator"); code != 0 {
		t.Fatalf("lowering the cap exited %d: %s", code, r.errb.String())
	}
	if code := r.do("set", "--scope", "user", capKey, "untrusted"); code != 0 || r.stored("user", capKey) != `"untrusted"` {
		t.Fatalf("lowering again exited %d", code)
	}
	refusals := r.auditCount("policy.refused")
	refused("raising the cap", r.do("set", "--json", "--scope", "user", capKey, "owner"))
	refused("raising it one step", r.do("set", "--json", "--scope", "user", capKey, "collaborator"))
	refused("unset of the cap", r.do("unset", "--json", "--scope", "user", capKey))
	refused("unset of another trust key", r.do("unset", "--json", "--scope", "user", capKey))
	refused("pin", r.do("pin", "--json", policyGit(t, r.work, "rev-parse", "HEAD")))
	refused("clear-pin", r.do("pin", "--json", "--clear"))
	if r.stored("user", capKey) != `"untrusted"` {
		t.Fatalf("the stored cap must be unchanged, got %s", r.stored("user", capKey))
	}
	if got := r.auditCount("policy.refused") - refusals; got != 6 {
		t.Fatalf("one audit row per refusal, got %d", got)
	}
	if pin, ok, _ := r.store().PolicyPinFor(context.Background(), r.project()); ok {
		t.Fatalf("a refused pin wrote %v", pin)
	}
	// A key outside trust.* is not restricted, and with NSELF_INVOKED_BY unset the operator's writes work as before.
	if code := r.do("set", "--scope", "project", schedpolicy.KeyPaidEnabled, "true"); code != 0 {
		t.Fatalf("a non-trust key stays writable, exit %d: %s", code, r.errb.String())
	}
	r.machine = false
	if code := r.do("set", "--scope", "user", capKey, "owner"); code != 0 || r.stored("user", capKey) != `"owner"` {
		t.Fatalf("the operator may raise the cap: exit %d", code)
	}
	if code := r.do("unset", "--scope", "user", capKey); code != 0 || r.stored("user", capKey) != "" {
		t.Fatalf("the operator may unset the cap: exit %d", code)
	}
}

func TestPolicyKeysRegisteredV15Only(t *testing.T) {
	for _, op := range []string{"show", "set", "unset", "pin"} {
		e, ok := v15Subcommands["policy "+op]
		if !ok || !e.V15Only {
			t.Fatalf("policy %s must be a V15Only command", op)
		}
		if _, v14 := subcommands["policy "+op]; v14 {
			t.Fatalf("policy %s must not exist in v1.4 argv", op)
		}
	}
}
