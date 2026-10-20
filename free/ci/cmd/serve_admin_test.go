package main

// Purpose: tests of `nself ci serve admin roles|token|audit` (P7-TRUST-12): token shown once and stored
// only as a digest, role grant/list/revoke, the audit verify/seal exit codes, and the group help.
// Inputs:  a real SQLite store in t.TempDir() with an injected clock, the commands' own run functions.
// Outputs: pass or fail; no network.
// Constraints: each test named by the Ticket's verify line asserts PASS and never skips.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

type adminRig struct {
	t      *testing.T
	path   string
	offset atomic.Int64
	out    bytes.Buffer
	errb   bytes.Buffer
}

func newAdminRig(t *testing.T) *adminRig {
	t.Helper()
	return &adminRig{t: t, path: filepath.Join(t.TempDir(), "state.db")}
}

func (r *adminRig) now() time.Time {
	return time.Unix(1_800_000_000, 0).Add(time.Duration(r.offset.Load()))
}

func (r *adminRig) env() adminEnv {
	r.out.Reset()
	r.errb.Reset()
	return adminEnv{Out: &r.out, Err: &r.errb, Open: func(ctx context.Context) (*roles.Service, roles.Actor, func(), error) {
		st, err := store.Open(r.path, store.Options{Clock: r.now})
		if err != nil {
			return nil, roles.Actor{}, nil, err
		}
		svc := roles.New(st, &audit.Log{St: st})
		svc.Now = r.now
		svc.Self = "alice"
		return svc, svc.Operator("cli:alice"), func() { _ = st.Close() }, nil
	}}
}

func (r *adminRig) data() map[string]any {
	r.t.Helper()
	var env struct {
		Data  map[string]any `json:"data"`
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal(r.out.Bytes(), &env); err != nil {
		r.t.Fatalf("not one JSON envelope: %v\n%s", err, r.out.String())
	}
	return env.Data
}

func TestServeAdminTokenIssueOnce(t *testing.T) {
	r := newAdminRig(t)
	if code := runAdminToken([]string{"issue", "bob", "--role", "release", "--project", "p1", "--project", "p2", "--ttl", "48h", "--json"}, r.env()); code != 0 {
		t.Fatalf("issue exited %d: %s %s", code, r.out.String(), r.errb.String())
	}
	d := r.data()
	tok, _ := d["token"].(string)
	if len(tok) != 43 || d["shown_once"] != true || d["role"] != "release" || d["subject"] != "bob" {
		t.Fatalf("issue data: %+v", d)
	}
	id, _ := d["id"].(string)
	for _, args := range [][]string{{"list", "--json"}, {"list"}} {
		if code := runAdminToken(args, r.env()); code != 0 {
			t.Fatalf("list exited %d", code)
		}
		if strings.Contains(r.out.String()+r.errb.String(), tok) || !strings.Contains(r.out.String(), id) {
			t.Fatalf("list must show the id and never the token: %s", r.out.String())
		}
	}
	// Text mode prints the token alone on stdout, once.
	if code := runAdminToken([]string{"issue", "carol", "--role", "member", "--project", "*"}, r.env()); code != 0 {
		t.Fatalf("text issue exited %d: %s", code, r.errb.String())
	}
	text := strings.TrimSpace(r.out.String())
	if len(text) != 43 || strings.Count(r.out.String()+r.errb.String(), text) != 1 {
		t.Fatalf("the token is printed once: out=%q err=%q", r.out.String(), r.errb.String())
	}
	// The store holds only digests: a grep of the database files finds neither token.
	r.env()
	for _, f := range []string{r.path, r.path + "-wal"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(b), tok) || strings.Contains(string(b), text) {
			t.Fatalf("%s holds a clear token", filepath.Base(f))
		}
	}
	db, err := sql.Open("sqlite", r.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err = db.QueryRow("SELECT COUNT(*) FROM trust_token WHERE length(digest)=64").Scan(&n); err != nil || n != 2 {
		t.Fatalf("two sha256 digests expected, got %d (%v)", n, err)
	}
	// Revoke, then a refused request and a usage error.
	if code := runAdminToken([]string{"revoke", id}, r.env()); code != 0 {
		t.Fatalf("revoke exited %d", code)
	}
	if code := runAdminToken([]string{"revoke", id}, r.env()); code != 1 {
		t.Fatalf("revoking twice exits 1, got %d", code)
	}
	if code := runAdminToken([]string{"issue", "bob", "--role", "root", "--project", "p", "--json"}, r.env()); code != 1 || !strings.Contains(r.out.String(), "E687") {
		t.Fatalf("an unknown role is E687: %d %s", code, r.out.String())
	}
	if code := runAdminToken([]string{"bogus"}, r.env()); code != 1 {
		t.Fatalf("unknown verb exits 1, got %d", code)
	}
}

func TestServeAdminRolesGrantListRevoke(t *testing.T) {
	r := newAdminRig(t)
	if code := runAdminRoles([]string{"grant", "bob", "--role", "release", "--project", "p1", "--json"}, r.env()); code != 0 {
		t.Fatalf("grant exited %d: %s", code, r.errb.String())
	}
	if code := runAdminRoles([]string{"grant", "alice", "--role", "admin", "--project", "*", "--self"}, r.env()); code != 0 {
		t.Fatalf("self grant exited %d: %s", code, r.errb.String())
	}
	if code := runAdminRoles([]string{"grant", "bob", "--role", "member", "--project", "p", "--self"}, r.env()); code != 1 {
		t.Fatalf("--self for another principal exits 1, got %d", code)
	}
	if code := runAdminRoles([]string{"list", "--json"}, r.env()); code != 0 {
		t.Fatalf("list exited %d", code)
	}
	grants, _ := r.data()["grants"].([]any)
	if len(grants) != 2 {
		t.Fatalf("two live grants expected: %s", r.out.String())
	}
	if code := runAdminRoles([]string{"revoke", "bob", "--json"}, r.env()); code != 0 {
		t.Fatalf("revoke exited %d", code)
	}
	if code := runAdminRoles([]string{"revoke", "bob"}, r.env()); code != 1 {
		t.Fatalf("revoking a missing grant exits 1, got %d", code)
	}
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 0 {
		t.Fatalf("grant and revoke rows verify: %d %s", code, r.errb.String())
	}
}

func TestServeAdminAuditSeal(t *testing.T) {
	r := newAdminRig(t)
	st, err := store.Open(r.path, store.Options{Clock: r.now})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"n1", "n2"} { // rows other packages write through the CI-31 API are not sealed
		if err = st.AppendAudit(ctx, "operator:cli:alice", "node.add", id, "added"); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 0 || !strings.Contains(r.out.String(), "unsealed: 2") {
		t.Fatalf("fresh unsealed rows are reported, not failed: %d %q", code, r.out.String())
	}
	r.offset.Store(int64(2 * time.Minute))
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 1 || !strings.Contains(r.out.String(), "unsealed: 2") || !strings.Contains(r.errb.String(), "unsealed: 2") {
		t.Fatalf("unsealed rows past the interval exit 1: %d %q %q", code, r.out.String(), r.errb.String())
	}
	if code := runAdminAudit([]string{"--verify", "--json"}, r.env()); code != 1 || r.data()["unsealed"].(float64) != 2 {
		t.Fatalf("the JSON verify reports unsealed 2 and exits 1: %d %s", code, r.out.String())
	}
	if code := runAdminAudit([]string{"--json"}, r.env()); code != 0 {
		t.Fatal("reading never seals and exits 0")
	}
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 1 {
		t.Fatalf("--verify must not seal, so it still fails: %d", code)
	}
	if code := runAdminAudit([]string{"seal", "--json"}, r.env()); code != 0 || r.data()["sealed"].(float64) != 2 {
		t.Fatalf("seal chains the two rows: %d %s", code, r.out.String())
	}
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 0 {
		t.Fatalf("a sealed log verifies: %d %q", code, r.errb.String())
	}
	db, err := sql.Open("sqlite", r.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec("UPDATE audit SET detail='forged' WHERE seq=1"); err != nil {
		t.Fatal(err)
	}
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 1 || !strings.Contains(r.errb.String(), "row 1") {
		t.Fatalf("an edited sealed row exits 1 naming the row: %d %q", code, r.errb.String())
	}
	if code := runAdminAudit([]string{"--since", "not-a-time"}, r.env()); code != 1 {
		t.Fatalf("a bad --since exits 1, got %d", code)
	}
	if code := runAdminAudit([]string{"--since", "24h"}, r.env()); code != 0 {
		t.Fatalf("--since 24h: %d", code)
	}
}

func TestServeAdminGroupRegistered(t *testing.T) {
	for _, k := range []string{"serve admin", "serve admin roles", "serve admin token", "serve admin audit"} {
		e, ok := v15Subcommands[k]
		if !ok || !e.V15Only || e.Flags == nil {
			t.Errorf("%q must be a V15-only command", k)
		}
		if _, v14 := subcommands[k]; v14 {
			t.Errorf("%q must not enter the v1.4 table", k)
		}
	}
	if len(tableProblems(v15Subcommands)) != 0 {
		t.Fatal(tableProblems(v15Subcommands))
	}
}

func TestServeAdminProductionEnvScrubsRegisteredValues(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	secret := "s3cr3t-project-value-0123456789"
	if err := cirun.RegisterSecretValue([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { audit.SetScanner(nil) })
	var out, errb bytes.Buffer
	env := productionAdminEnv()
	env.Out, env.Err = &out, &errb
	if code := runAdminRoles([]string{"grant", "bob", "--role", "member", "--project", secret, "--json"}, env); code != 1 || !strings.Contains(out.String(), "E687") {
		t.Fatalf("a registered value in a mutation's audit row is refused: %d %s", code, out.String())
	}
	if strings.Contains(out.String()+errb.String(), secret) {
		t.Fatal("the refusal must not echo the value")
	}
	out.Reset()
	if code := runAdminRoles([]string{"list", "--json"}, env); code != 0 || strings.Contains(out.String(), "bob") {
		t.Fatalf("nothing was granted: %d %s", code, out.String())
	}
}

func TestServeAdminAuditTextIsPlain(t *testing.T) {
	r := newAdminRig(t)
	st, err := store.Open(r.path, store.Options{Clock: r.now})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.AppendAudit(context.Background(), "operator:x", "node.add", "n1", "line one\nforged\tline \x1b[31mred"); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if code := runAdminAudit(nil, r.env()); code != 0 {
		t.Fatal(code)
	}
	if strings.Count(r.out.String(), "\n") != 1 || strings.ContainsAny(r.out.String(), "\x1b") || strings.Count(r.out.String(), "\t") != 5 {
		t.Fatalf("one plain line per row expected: %q", r.out.String())
	}
}

func TestServeAdminRevokeEndsTokensToo(t *testing.T) {
	r := newAdminRig(t)
	if code := runAdminToken([]string{"issue", "bob", "--role", "member", "--project", "p", "--json"}, r.env()); code != 0 {
		t.Fatal(code)
	}
	if code := runAdminRoles([]string{"revoke", "bob"}, r.env()); code != 0 {
		t.Fatalf("a principal that holds only a token can be revoked: %d %s", code, r.errb.String())
	}
	if code := runAdminToken([]string{"list", "--json"}, r.env()); code != 0 || !strings.Contains(r.out.String(), `"revoked"`) {
		t.Fatalf("the token is revoked: %s", r.out.String())
	}
}

// TestServeAdminRevokeRefusesWhileARestoreIsPending: `roles revoke` and `token revoke` write into the data
// dir, so a half-restored dir (restore.pending) refuses them, as `serve` and `serve backup` do (D-1307).
func TestServeAdminRevokeRefusesWhileARestoreIsPending(t *testing.T) {
	r := newAdminRig(t)
	if code := runAdminToken([]string{"issue", "bob", "--role", "member", "--project", "p1", "--json"}, r.env()); code != 0 {
		t.Fatalf("issue exited %d: %s", code, r.errb.String())
	}
	id, _ := r.data()["id"].(string)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "restore.pending"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := r.env()
	env.DataDir = home
	if code := runAdminToken([]string{"revoke", id}, env); code == 0 || !strings.Contains(r.errb.String(), "incomplete_restore") {
		t.Fatalf("token revoke exited %d with %q: want a refusal naming incomplete_restore", code, r.errb.String())
	}
	env = r.env()
	env.DataDir = home
	if code := runAdminRoles([]string{"revoke", "bob"}, env); code == 0 || !strings.Contains(r.errb.String(), "incomplete_restore") {
		t.Fatalf("roles revoke exited %d with %q: want a refusal naming incomplete_restore", code, r.errb.String())
	}
	if err := os.Remove(filepath.Join(home, "restore.pending")); err != nil {
		t.Fatal(err)
	}
	env = r.env()
	env.DataDir = home
	if code := runAdminToken([]string{"revoke", id}, env); code != 0 {
		t.Fatalf("token revoke exited %d once the marker was gone: %s", code, r.errb.String())
	}
}
