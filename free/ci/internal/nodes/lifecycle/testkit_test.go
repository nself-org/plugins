package lifecycle

// Purpose: shared fixtures of the lifecycle unit tests: a Deps whose ssh, scp and probe seams are fakes,
// an ELF fixture, an audit counter and the fake secret set the canary tests look for.
// Inputs:  none. Outputs: helpers only. Constraints: every secret-looking value is synthetic.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"debug/elf"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

// keyA and keyB are two well-formed ed25519 host keys; fpA and fpB are their OpenSSH fingerprints.
var (
	keyA, keyB = edKey(1), edKey(2)
	fpA, fpB   = keyA.Fingerprint, keyB.Fingerprint
)

func edKey(seed byte) remote.HostKey {
	blob := []byte{0, 0, 0, 11}
	blob = append(blob, "ssh-ed25519"...)
	blob = append(blob, 0, 0, 0, 32)
	blob = append(blob, bytes.Repeat([]byte{seed}, 32)...)
	sum := sha256.Sum256(blob)
	return hostKey("ssh-ed25519", "SHA256:"+base64.RawStdEncoding.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(blob))
}

// secretFixtures are fake values no output and no audit row may contain.
// Built at run time so no secret-shaped literal sits in the source or its history.
var secretFixtures = []string{"FAKE-SECRET-CANARY-0001", "gh" + "p_" + strings.Repeat("F", 36), "sk" + "_test_" + strings.Repeat("F", 24)}

type kit struct {
	t       *testing.T
	d       *Deps
	st      *store.Store
	dbPath  string
	out     *bytes.Buffer
	mu      sync.Mutex
	keys    []remote.HostKey
	runs    [][]string
	copies  []string
	failRun map[string]error
	uname   string
	// mutate lets a test change what the fake prober reports.
	mutate func(*model.Capability)
	// forced records, per probe, whether the caller asked to skip the probe cache.
	forced []bool
}

func hostKey(typ, fp, b64 string) remote.HostKey {
	return remote.HostKey{Type: typ, Fingerprint: fp, Line: "node " + typ + " " + b64}
}

// newKit builds Deps over a fresh store with fakes; project is "proj-1" and the actor "cli:test".
func newKit(t *testing.T) *kit {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "state.db")
	st, err := store.Open(dbPath, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	k := &kit{t: t, st: st, dbPath: dbPath, out: &bytes.Buffer{}, failRun: map[string]error{}, uname: "Linux x86_64",
		keys: []remote.HostKey{keyA}}
	d := NewDeps(st, home, "proj-1", "cli:test")
	d.Stdin, d.Stdout, d.Stderr, d.TTY = strings.NewReader(""), k.out, k.out, false
	d.SSHVersion = func(context.Context) (remote.Version, error) { return remote.Version{}, nil }
	d.Scan = func(context.Context, remote.Target, string) ([]remote.HostKey, error) { return k.keys, nil }
	d.Run = k.run
	d.Copy = func(_ context.Context, _ remote.Target, local, _ string) error {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.copies = append(k.copies, local)
		return nil
	}
	d.NewProber = func(r *registry.Registry, _ string, _ remote.Version) probe.Prober { return fakeProber{r, k} }
	d.Separation = fixedSeparation{SeparationFacts{UIDSeparation: true, UserNS: true, Reason: "userns.ok"}}
	k.d = d
	return k
}

func (k *kit) run(_ context.Context, _ remote.Target, argv ...string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.runs = append(k.runs, argv)
	if err := k.failRun[argv[0]]; err != nil {
		return "", err
	}
	switch {
	case argv[0] == "uname":
		return k.uname + "\n", nil
	case argv[0] == agentPath && len(argv) > 1 && argv[1] == "version":
		return "nself-ci-agent 1.5.0 linux/amd64\n", nil
	}
	return "", nil
}

type fakeProber struct {
	r *registry.Registry
	k *kit
}

func (p fakeProber) Probe(ctx context.Context, id string) (registry.Node, error) {
	p.k.mu.Lock()
	p.k.forced = append(p.k.forced, probe.Forced(ctx))
	p.k.mu.Unlock()
	n, err := p.r.Get(ctx, id)
	if err == nil && p.k.mutate != nil {
		p.k.mutate(&n.Capability)
	}
	return n, err
}

type fixedSeparation struct{ f SeparationFacts }

func (s fixedSeparation) Detect(context.Context, func(context.Context, ...string) (string, error), Platform) SeparationFacts {
	return s.f
}

// elfFixture writes a header-only ELF executable for the machine and returns its path.
func elfFixture(t *testing.T, m elf.Machine) string {
	t.Helper()
	h := make([]byte, 64)
	copy(h, "\x7fELF")
	h[4], h[5], h[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(h[16:], uint16(elf.ET_EXEC))
	binary.LittleEndian.PutUint16(h[18:], uint16(m))
	binary.LittleEndian.PutUint32(h[20:], 1)
	binary.LittleEndian.PutUint16(h[52:], 64)
	p := filepath.Join(t.TempDir(), "nself-ci-agent")
	if err := os.WriteFile(p, h, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// auditCount counts audit rows with the action, or all rows when action is "".
func (k *kit) auditCount(action string) int {
	k.t.Helper()
	db, err := sql.Open("sqlite", "file:"+k.dbPath+"?mode=ro")
	if err != nil {
		k.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	q, args := "SELECT count(*) FROM audit", []any{}
	if action != "" {
		q, args = q+" WHERE action=?", []any{action}
	}
	var n int
	if err = db.QueryRow(q, args...).Scan(&n); err != nil {
		k.t.Fatal(err)
	}
	return n
}

// auditText is every audit field concatenated, for the canary checks.
func (k *kit) auditText() string {
	k.t.Helper()
	db, err := sql.Open("sqlite", "file:"+k.dbPath+"?mode=ro")
	if err != nil {
		k.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Query("SELECT actor||' '||action||' '||target||' '||detail FROM audit")
	if err != nil {
		k.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var b strings.Builder
	for rows.Next() {
		var s string
		if err = rows.Scan(&s); err != nil {
			k.t.Fatal(err)
		}
		b.WriteString(s + "\n")
	}
	return b.String()
}

func (k *kit) addNode(name string) AddResult {
	k.t.Helper()
	r, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@" + name, HostKey: fpA,
		Source: LocalFile{Path: elfFixture(k.t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	if err != nil {
		k.t.Fatalf("add: %v", err)
	}
	return r
}

func errCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || CodeOf(err) != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}

var errBoom = errors.New("boom")

// resetExtensions drops what a test registered and restores the built-ins.
func resetExtensions() {
	ext = newExtensions()
	RegisterAgentInstaller(DefaultInstaller, localFileInstaller)
	RegisterRotateMode(HostKeyMode, rotateHostKey)
}

// runWith runs one subcommand through Run against d.
func runWith(d *Deps, sub string, args ...string) (code int, out, errs string) {
	var o, e bytes.Buffer
	env := Env{Out: &o, Err: &e, Open: func(context.Context, string) (*Deps, func(), error) { return d, func() {}, nil }}
	code = Run(sub, args, env)
	return code, o.String(), e.String()
}
