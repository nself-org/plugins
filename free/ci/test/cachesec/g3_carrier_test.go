//go:build cachesec

package cachesec

// Purpose: group g3 in-process carriers. Carrier "ssh": the real ssh driver on the coordinator and a real
// agent session on the node, joined by pipes. Carrier "agent": the real agent pull driver and agentapi
// listener (TLS 1.3 mTLS, token gate, lease engine) and a real agent session over the real pull carrier.
// The node's After hook is scripted per case (g3step): it sends the blobs and refs a malicious or honest
// node would, with the same transfer.Sender the shipped agent uses.
// Inputs: a g3rig and a g3node. Outputs: the driver result; frames land in the rig's g3Sink.
// Constraints: documentation addresses only; no network beyond 127.0.0.1; frames are never patched on the
// coordinator side, a node-side tap may corrupt a chunk in transit.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdkremote "github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/agentapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/store"
	agenttr "github.com/nself-org/plugins/free/ci/internal/transport/agent"
	sshdrv "github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

// g3step is one thing a node does after its job passed.
type g3step func(ctx context.Context, l protocol.Lease, up session.Uploader) error

// g3node scripts the node end of a lease.
type g3node struct {
	steps   []g3step
	onFirst func() // runs once, before the first step (a case expires the lease here)
	flip    int    // flip a byte of the Nth chunk frame sent (1-based, 0 = never): corruption in transit
	once    sync.Once
	mu      sync.Mutex
	chunks  int
	errs    []error // what the node's own sends returned (the coordinator's verdict as the node saw it)
}

func g3Sum(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

// g3Send sends one blob of the stream (cache|artifact) and waits for the receiver's verdict.
func g3Send(stream string, data []byte) g3step {
	return func(ctx context.Context, _ protocol.Lease, up session.Uploader) error {
		s := transfer.Sender{Up: up, Offsets: up.(session.OffsetSource).Offsets(), WaitOffset: time.Second, MaxResumes: 1}
		return s.Send(ctx, transfer.Blob{Stream: stream, SHA256: g3Sum(data), Size: int64(len(data)),
			Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil }})
	}
}

// g3CacheRef sends a cache-ref for spec "go" with the given blobs (path -> data).
func g3CacheRef(key string, blobs ...[2]any) g3step {
	return func(_ context.Context, l protocol.Lease, up session.Uploader) error {
		ref := protocol.CacheRef{LeaseID: l.LeaseID, Spec: "go", Key: key}
		for _, b := range blobs {
			d := b[1].([]byte)
			ref.Blobs = append(ref.Blobs, protocol.CacheBlob{Path: b[0].(string), SHA256: g3Sum(d), Size: int64(len(d))})
		}
		return up.CacheRef(ref)
	}
}

// g3ArtRef sends an artifact-ref for path naming data.
func g3ArtRef(path string, data []byte) g3step {
	return func(_ context.Context, l protocol.Lease, up session.Uploader) error {
		return up.ArtifactRef(protocol.ArtifactRef{LeaseID: l.LeaseID, Path: path, SHA256: g3Sum(data), Size: int64(len(data))})
	}
}

func (n *g3node) runner() session.Runner {
	return session.RunnerFunc(func(context.Context, session.JobRun) (session.JobResult, error) {
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: json.RawMessage(`{"schema":"test","ok":true}`)}, nil
	})
}

func (n *g3node) hooks() []session.JobHook {
	return []session.JobHook{{Name: "g3", After: func(ctx context.Context, l protocol.Lease, _ session.JobRoot, out session.Outcome, up session.Uploader) error {
		if out.State != "passed" {
			return nil
		}
		if n.onFirst != nil {
			n.once.Do(n.onFirst)
		}
		for _, st := range n.steps {
			if err := st(ctx, l, up); err != nil {
				n.mu.Lock()
				n.errs = append(n.errs, err)
				n.mu.Unlock()
				return fmt.Errorf("%w: %v", session.ErrHookFatal, err)
			}
		}
		return nil
	}}}
}

// tap flips one byte of the configured chunk frame before it leaves the node.
func (n *g3node) tap(c session.Carrier) session.Carrier { return &g3tap{Carrier: c, n: n} }

type g3tap struct {
	session.Carrier
	n *g3node
}

func (c *g3tap) Send(m protocol.Message) error {
	if ch, ok := m.Body.(protocol.Chunk); ok {
		c.n.mu.Lock()
		c.n.chunks++
		hit := c.n.flip != 0 && c.n.chunks == c.n.flip
		c.n.mu.Unlock()
		if hit {
			b := []byte(ch.Data)
			b[len(b)/2] ^= 0x01
			ch.Data = string(b)
			m.Body = ch
		}
	}
	return c.Carrier.Send(m)
}

type g3fakeKey struct{}

func (g3fakeKey) KeyID() string        { return "k1" }
func (g3fakeKey) PublicString() string { return "pub" }

// RunSSH delivers the rig's lease to the node over the ssh driver (pipes stand in for the ssh channel).
func (r *g3rig) RunSSH(n *g3node, sink *g3Sink) error {
	r.t.Helper()
	home := r.t.TempDir()
	pin := sdkremote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
	if err := pin.Add("nself-ci-node1", "192.0.2.5 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
		r.t.Fatal(err)
	}
	var capab model.Capability
	capab.Identity.ID, capab.Identity.Transport = "node1", sshdrv.Kind
	op := "operator"
	capab.Identity.Ownership.Value = &op
	capab.SecretsEligible = true
	capab.Identity.SSH = &model.CapabilitySSH{Hostname: "192.0.2.5", Port: 2222, User: "ci"}
	state := r.t.TempDir()
	var once sync.Once
	starter := func(ctx context.Context, _ sdkremote.Target, _ string) (*sshdrv.Conn, error) {
		cr, cw := io.Pipe()
		ar, aw := io.Pipe()
		er, ew := io.Pipe()
		table, err := slots.Open(state, 2)
		if err != nil {
			return nil, err
		}
		actx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			car, err := session.NewStreamCarrier(cr, aw, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
			if err == nil {
				var s *session.Session
				s, err = session.New(session.Config{Carrier: n.tap(car), Key: g3fakeKey{}, Slots: table, Runner: n.runner(), StateDir: state, Hooks: n.hooks(),
					Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: "m1"}})
				if err == nil {
					err = s.Run(actx)
				}
			}
			_ = aw.Close()
			_ = ew.Close()
			done <- err
		}()
		return &sshdrv.Conn{Stdin: cw, Stdout: ar, Stderr: er, Wait: func() error { <-done; return nil },
			Kill: func() error { once.Do(cancel); _ = cr.Close(); _ = aw.Close(); return nil }}, nil
	}
	d := &sshdrv.Driver{Home: home, Start: starter, SSHVersion: func(context.Context) (sdkremote.Version, error) { return sdkremote.Version{Major: 9, Minor: 6}, nil },
		NoRenew: true, Heartbeat: time.Second, AgentTTL: 25 * time.Second, CancelGrace: 3 * time.Second, HelloTimeout: 10 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return d.Run(ctx, capab, r.Proto(), sink)
}

type g3nodeKey struct{ pub ed25519.PublicKey }

func (k g3nodeKey) KeyID() string        { return pki.NodeKeyID(k.pub) }
func (k g3nodeKey) PublicString() string { return base64.StdEncoding.EncodeToString(k.pub) }

// RunAgent delivers the rig's lease to the node over the agent pull carrier (mTLS on 127.0.0.1).
func (r *g3rig) RunAgent(n *g3node, sink *g3Sink) error {
	r.t.Helper()
	t, st := r.t, r.w.St
	ctx := context.Background()
	ca, err := pki.New(ctx, pki.Options{Store: st, DataDir: t.TempDir(), Now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := ca.Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	drv := &agenttr.Driver{Gate: &agenttr.Gate{Tokens: tokens, Bindings: st, Leases: st}, Minter: tokens, Leases: st, LogDir: t.TempDir(), Poll: 30 * time.Millisecond, CancelGrace: 2 * time.Second}
	leaf := &agentapi.LeafSource{CA: ca, ID: "coord-1", IPs: []net.IP{net.ParseIP("127.0.0.1")}}
	srv, err := agentapi.New(agentapi.Config{CA: ca, Guard: pki.NewGuard(st), Cert: leaf.Get, Leases: r.eng, Sink: drv.Sink(), Source: drv, Store: st,
		Admit: drv.Admit, Blobs: drv.Blobs(), PollTimeout: 2 * time.Second, SourcePoll: 20 * time.Millisecond, GuardEvery: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	drv.SetCancel(srv.Cancel)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
		<-done
	}()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	b, err := os.ReadFile("../../internal/model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var capab model.Capability
	if err = json.Unmarshal(b, &capab); err != nil {
		t.Fatal(err)
	}
	capab.Identity.ID, capab.Identity.Name, capab.Identity.Transport = r.node, r.node, agenttr.Kind
	capab.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{"proj-1"}}
	doc, _ := json.Marshal(capab)
	admin, _ := store.NewAdminAuthority("cli:test")
	if err = st.NodeAdd(ctx, store.NodeRecord{ID: r.node, Name: r.node, State: "online", Reason: "added", Capability: doc}, admin); err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "x"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	is, err := ca.IssueNode(ctx, pki.NodeRequest{CSR: csr, NodeID: r.node, OfferID: "offer-" + r.node})
	if err != nil {
		t.Fatal(err)
	}
	leafCert, _ := x509.ParseCertificate(is.CertDER)
	chain := append(append([][]byte{}, is.Chain...), is.RootDER)
	cert := tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: leafCert}
	tlsc, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: ca.RootDER(), Cert: &cert, ServerName: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	car := agenttr.NewCarrier(agenttr.CarrierConfig{Client: agenttr.Options{BaseURL: "https://" + ln.Addr().String(), TLS: tlsc, Wait: time.Second},
		Backoff: agenttr.Backoff{Min: 10 * time.Millisecond, Max: 50 * time.Millisecond}})
	t.Cleanup(func() { _ = car.Close() })
	state := t.TempDir()
	table, err := slots.Open(state, 2)
	if err != nil {
		t.Fatal(err)
	}
	s, err := session.New(session.Config{Carrier: n.tap(car), Key: g3nodeKey{pub}, Slots: table, Runner: n.runner(), StateDir: state, Hooks: n.hooks(),
		Coordinator: session.Coordinator{ID: "coord-1", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: "m-1", CapabilityDigest: "sha256:00"}})
	if err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	sessDone := make(chan error, 1)
	go func() { sessDone <- s.Run(rctx) }()
	defer func() { cancel(); <-sessDone }()
	return drv.Run(rctx, capab, r.Proto(), sink)
}
