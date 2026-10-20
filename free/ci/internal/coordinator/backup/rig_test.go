package backup

// Purpose: shared fixtures of the backup tests: a data dir with a real SQLite store, a CA (and so a KEK), two
//          nodes with certificates and pinned keys, a queued and a leased attempt, evidence, a job token.
// Inputs:  none. Outputs: helpers only.
// Constraints: all time comes from the rig clock; every key is generated per test; synthetic values only
//          (documentation address ranges, no real host, no real secret).

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var t0 = time.Unix(1_800_000_000, 0)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }

type rig struct {
	t      *testing.T
	dir    string
	clk    *clock
	st     *store.Store
	ca     *pki.CA
	kek    []byte
	leaves map[string]*x509.Certificate
	token  string
	epochs []int64 // every lease epoch granted before the backup
	coord  store.Coordinator
}

func (r *rig) opts() Options {
	return Options{DataDir: r.dir, Clock: r.clk.now, Alive: func(int) bool { return false }}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dir := t.TempDir()
	clk := &clock{t: t0}
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{Clock: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, dir: dir, clk: clk, st: st, leaves: map[string]*x509.Certificate{}}
	t.Cleanup(func() { _ = r.st.Close() })
	if r.ca, err = pki.New(context.Background(), pki.Options{Store: st, DataDir: dir, Now: clk.now}); err != nil {
		t.Fatal(err)
	}
	if r.kek, err = pki.LoadKEK(dir, false); err != nil {
		t.Fatal(err)
	}
	return r
}

// addNode registers a node (state online), issues it a certificate and pins an Ed25519 key.
func (r *rig) addNode(id string) {
	r.t.Helper()
	ctx := context.Background()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		r.t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		r.t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name = id, id
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(c)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = r.st.NodeAdd(ctx, store.NodeRecord{ID: id, Name: id, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		r.t.Fatal(err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err = r.ca.PinNodeKey(ctx, id, pub); err != nil {
		r.t.Fatal(err)
	}
}

// pair issues the node a certificate, as enrollment does.
func (r *rig) pair(id string) {
	r.t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: id}}, key)
	if err != nil {
		r.t.Fatal(err)
	}
	is, err := r.ca.IssueNode(context.Background(), pki.NodeRequest{CSR: csr, NodeID: id, OfferID: "offer-" + id})
	if err != nil {
		r.t.Fatal(err)
	}
	if r.leaves[id], err = x509.ParseCertificate(is.CertDER); err != nil {
		r.t.Fatal(err)
	}
}

// seedWork writes a queued attempt, a leased one (epoch of incarnation 2), evidence and a job token.
func (r *rig) seedWork() {
	r.t.Helper()
	ctx := context.Background()
	var err error
	if r.coord, err = r.st.RegisterCoordinator(ctx, 4242, "start", "boot"); err != nil {
		r.t.Fatal(err)
	}
	if r.coord, err = r.st.RegisterCoordinator(ctx, 4242, "start", "boot"); err != nil { // same identity again: incarnation 2
		r.t.Fatal(err)
	}
	must := func(err error) {
		r.t.Helper()
		if err != nil {
			r.t.Fatal(err)
		}
	}
	must(r.st.CreatePipeline(ctx, store.PipelineRow{ID: "p1", Project: "proj-1", Revision: "rev-1", Trigger: "push", SourceTrust: "owner", PrivacyZone: "private", PolicyDigest: "pd", InputDigest: "id", SelectionMode: "all", SelectionReason: "r", SelectionDigest: "sd", Status: "running"}))
	for _, j := range []string{"j-queued", "j-leased"} {
		must(r.st.CreateJob(ctx, store.JobRow{ID: j, PipelineID: "p1", Name: j, Kind: "test", InputDigest: "x", Idempotent: true, Required: true, InfraMax: 2}))
		must(r.st.CreateAttempt(ctx, store.AttemptRow{ID: "a-" + j, JobID: j, N: 1, CoordinatorID: r.coord.ID}, ""))
	}
	epoch, err := r.st.LeaseGrant(ctx, "host-1", store.Demand{AttemptID: "a-j-leased", LeaseID: "lease-1", RunnerID: "node-a", CPU: 1, MemMB: 1, CapacityCPU: 4, CapacityMemMB: 4, TTL: time.Minute}, r.coord.Incarnation)
	must(err)
	r.epochs = append(r.epochs, epoch)
	must(r.st.RecordEvidence(ctx, "ev-1", "p1", "a-j-queued", "rev-1", "in-1", []byte(`{"result":"ok"}`)))
	tk, err := r.ca.Tokens(ctx)
	must(err)
	r.token, err = tk.Mint(pki.TokenClaims{JTI: "jti-1", JobID: "j-leased", AttemptID: "a-j-leased", Revision: "rev-1", RunnerID: "node-a", Epoch: epoch}, time.Hour)
	must(err)
}

// archive takes a backup of the rig's data dir.
func (r *rig) archive() []byte {
	r.t.Helper()
	var buf bytes.Buffer
	if _, err := Create(context.Background(), r.opts(), &buf); err != nil {
		r.t.Fatal(err)
	}
	return buf.Bytes()
}

// reopen opens the store of dir with the rig clock and a CA over it.
func (r *rig) reopen(dir string) (*store.Store, *pki.CA) {
	r.t.Helper()
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{Clock: r.clk.now})
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = st.Close() })
	ca, err := pki.New(context.Background(), pki.Options{Store: st, DataDir: dir, Now: r.clk.now})
	if err != nil {
		r.t.Fatal(err)
	}
	return st, ca
}

func wantReason(t *testing.T, err error, reason string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want E668 %s", reason)
	}
	if got := ReasonOf(err); got != reason {
		t.Fatalf("reason %q (%v), want %q", got, err, reason)
	}
}

func dirEmpty(t *testing.T, dir string) {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil || len(es) != 0 {
		names := []string{}
		for _, e := range es {
			names = append(names, e.Name())
		}
		t.Fatalf("data dir was touched: %v (%v)", names, err)
	}
}
