package main

// Purpose: P7-WIRE-01. `serve` (the TEAM coordinator) with its default flags places an ordinary job on an
// enrolled node, and a job no node can take fails with E618 or E693 instead of waiting.
// Inputs:  the command root driven by argv (runV15, the V15 entry of main), a stored intake row, a git repository with a one-job config and
//          a registered ssh node whose agent is the in-process session the shipped agent runs.
// Outputs: pass or fail.
// Constraints: nothing is injected into the composition under test; the one seam used is the forge clone
//          (dispatchCloneRef), which here hands out a checkout of the local repository. NSELF_V15=1, no
//          docker, no network, no real ssh; ssh.Default is restored afterwards; tests do not run in parallel.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/slots"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	_ "github.com/nself-org/plugins/free/ci/internal/schedtel" // the binary registers the telemetry sink and finalizer
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport/ssh"
)

type wire01Key struct{}

func (wire01Key) KeyID() string        { return "k1" }
func (wire01Key) PublicString() string { return "pub" }

// wire01Node is one registered ssh node and the in-process agent behind it.
type wire01Node struct {
	id, host, state string
	mu              sync.Mutex
	leases          []protocol.Lease
}

func (n *wire01Node) ran() []protocol.Lease {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]protocol.Lease(nil), n.leases...)
}

func (n *wire01Node) serve(ctx context.Context, table *slots.Table, in io.Reader, out io.Writer) error {
	car, err := session.NewStreamCarrier(in, out, protocol.CarrierPolicy{Carrier: "A", Personal: true, OperatorOwned: true}, true)
	if err != nil {
		return err
	}
	runner := session.RunnerFunc(func(_ context.Context, j session.JobRun) (session.JobResult, error) {
		n.mu.Lock()
		n.leases = append(n.leases, j.Lease)
		n.mu.Unlock()
		ev := model.Evidence{Schema: "ci.evidence/v1", Revision: j.Lease.Checkout.Revision, Attempt: &model.AttemptRef{ID: j.Lease.AttemptID, N: 1}, Result: "pass"}
		raw, _ := json.Marshal(ev)
		return session.JobResult{Outcome: session.Outcome{State: "passed"}, Evidence: raw}, nil
	})
	s, err := session.New(session.Config{Carrier: car, Key: wire01Key{}, Slots: table, Runner: runner, StateDir: n.state, Hooks: []session.JobHook{},
		Coordinator: session.Coordinator{ID: "coord", RootFingerprint: "fp"}, Facts: session.Facts{AgentVersion: "1.5.0", MachineID: n.id}})
	if err != nil {
		return err
	}
	return s.Run(ctx)
}

func wire01Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// wire01Fixture is a repository with one ordinary job, a coordinator home and, when nodes is true, one enrolled node.
func wire01Fixture(t *testing.T, nodes bool, maxQueue string) (repo, home, sha string, node *wire01Node) {
	t.Helper()
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_STORE_MIN_FREE", "1B")
	repo, home = t.TempDir(), t.TempDir()
	wire01Git(t, repo, "init", "-q")
	if err := os.MkdirAll(filepath.Join(repo, ".nself"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte("version: 1\njobs:\n  unit:\n    kind: test\n    run: [\"true\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wire01Git(t, repo, "add", ".")
	wire01Git(t, repo, "commit", "-q", "-m", "init")
	sha = wire01Git(t, repo, "rev-parse", "HEAD")
	ctx := context.Background()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if nodes {
		node = &wire01Node{id: "n1", host: "10.0.0.5", state: t.TempDir()}
		pin := remote.PinnedHostKeys{Path: filepath.Join(home, "known_hosts")}
		if err = pin.Add("nself-ci-n1", node.host+" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDummyKeyForTestsOnlyDummyKeyForTestsOnly000"); err != nil {
			t.Fatal(err)
		}
		c, err := planview.LocalCapability(planview.LocalFacts{Project: repo, Platform: runtime.GOOS + "/" + runtime.GOARCH, CPUs: 8, MemMB: 8192, Now: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		lan, owned, uid := "lan", "owned", true
		c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = node.id, node.id, "ssh", "ssh"
		c.Identity.SSH = &model.CapabilitySSH{Hostname: node.host, Port: 22, User: "ci"}
		c.SecretsEligible = true
		docker := true // the node can honour the container isolation serve's default --container asks for
		c.Tools.Docker.Value = &docker
		c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
		c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
		trust := model.CapabilityTrust{
			Accepts:       wire01Assigned([]model.TrustClass{"owner", "internal"}),
			Isolation:     wire01Assigned(model.Isolation("container")),
			Network:       wire01Assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
			SecretClasses: wire01Assigned([]model.SecretClass{"none"}),
			PrivacyZone:   wire01Assigned(model.PrivacyZone("private-infrastructure")),
		}
		c.Trust = trust
		doc, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if err = st.NodePut(ctx, store.NodeRecord{ID: node.id, Name: node.id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
			t.Fatal(err)
		}
		admin, err := store.NewAdminAuthority("test")
		if err != nil {
			t.Fatal(err)
		}
		reg := registry.New(st)
		if _, err = reg.SetTrust(ctx, node.id, trust, admin); err != nil {
			t.Fatal(err)
		}
		if _, err = reg.Authorize(ctx, node.id, "o/r", "test"); err != nil { // the project id under serve is the forge repo name
			t.Fatal(err)
		}
		if _, err = reg.SetState(ctx, node.id, "online", "", "test"); err != nil {
			t.Fatal(err)
		}
		d := ssh.Default
		was := struct {
			home, repo, min string
			start           ssh.Starter
			ver             func(context.Context) (remote.Version, error)
			renew           func(context.Context, string, uint64) error
			personal        bool
			hb, ttl, grace  time.Duration
		}{d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace}
		d.Home, d.RepoDir, d.MinAgentVersion = home, repo, "1.5.0"
		d.Start = func(_ context.Context, tgt remote.Target, _ string) (*ssh.Conn, error) {
			if !strings.HasSuffix(tgt.Dest, "@"+node.host) {
				return nil, fmt.Errorf("no node at %s", tgt.Dest)
			}
			toAgentR, toAgentW := io.Pipe()
			fromAgentR, fromAgentW := io.Pipe()
			errR, errW := io.Pipe()
			table, err := slots.Open(node.state, 2)
			if err != nil {
				return nil, err
			}
			actx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() {
				e := node.serve(actx, table, toAgentR, fromAgentW)
				_ = fromAgentW.Close()
				_ = errW.Close()
				done <- e
			}()
			kill := func() error {
				_ = fromAgentW.Close()
				_ = toAgentR.Close()
				cancel()
				return nil
			}
			return &ssh.Conn{Stdin: toAgentW, Stdout: fromAgentR, Stderr: errR, Wait: func() error { <-done; return nil }, Kill: kill}, nil
		}
		d.SSHVersion = func(context.Context) (remote.Version, error) { return remote.Version{Major: 9, Minor: 6}, nil }
		d.Renew, d.Personal, d.Heartbeat, d.AgentTTL, d.CancelGrace = cirun.RenewLease, true, time.Second, 25*time.Second, 3*time.Second
		t.Cleanup(func() {
			d.Home, d.RepoDir, d.MinAgentVersion, d.Start, d.SSHVersion, d.Renew, d.Personal = was.home, was.repo, was.min, was.start, was.ver, was.renew, was.personal
			d.Heartbeat, d.AgentTTL, d.CancelGrace = was.hb, was.ttl, was.grace
		})
	}
	if maxQueue != "" { // the project's queue limit, so the probe does not wait out the 10 minute default
		err = st.ApplyWrite(ctx, store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: schedpolicy.KeyMaxQueue, Value: []byte(maxQueue), Actor: "test"}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	// the submitted run: an admin's API submission, exactly as the user API stores it
	svc := roles.New(st, &audit.Log{St: st})
	if err = svc.Grant(ctx, svc.Operator("local"), roles.Request{Principal: "bob", Role: roles.Admin, Projects: []string{"o/r"}}); err != nil {
		t.Fatal(err)
	}
	var d coordinator.Delivery
	backend := clientapi.StoreBackend{Enqueue: func(_ context.Context, got coordinator.Delivery) (coordinator.Outcome, error) {
		d = got
		return coordinator.Outcome{}, nil
	}}
	if _, err = backend.Submit(ctx, clientapi.SubmitRequest{Project: "o/r", Revision: sha}, "user:bob"); err != nil {
		t.Fatal(err)
	}
	facts := d.Facts
	if _, err = st.IntakeEnqueue(ctx, store.IntakeRequest{Source: d.Source, Event: "api", DeliveryID: "d1", Project: d.Project, Revision: d.Revision, TriggerClass: "change",
		PolicyDigest: "default", SemanticKey: "k1", Facts: facts}); err != nil {
		t.Fatal(err)
	}
	return repo, home, sha, node
}

func wire01Assigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

// wire01Serve runs `serve` with default flags through the cobra root until the queued row is finished,
// then stops it with SIGTERM. It returns the finished row.
func wire01Serve(t *testing.T, repo, home string) store.IntakeRow {
	t.Helper()
	clone := dispatchCloneRef
	dispatchCloneRef = func(_, _, workdir string) (string, error) {
		dir, err := os.MkdirTemp(workdir, "co-")
		if err != nil {
			return "", err
		}
		if out, err := exec.Command("git", "clone", "-q", repo, dir).CombinedOutput(); err != nil {
			return "", fmt.Errorf("%v: %s", err, out)
		}
		return dir, nil
	}
	// the other seam: the forge. The trust query would call api.github.com, so its answer is fixed here; the run itself stays real.
	execute := dispatchRun
	dispatchRun = func(ctx context.Context, c *cirun.Coordinator) (cirun.Result, error) {
		tr, ok := c.Hooks.Trust.(forgeDispatchTrust)
		if !ok {
			t.Errorf("dispatch did not install forge trust: %T", c.Hooks.Trust)
			return execute(ctx, c)
		}
		tr.hook.Refetch = func(_ context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
			f.SHAReachable = apiDispatchFact(true)
			return f, nil
		}
		c.Hooks.Trust = tr
		return execute(ctx, c)
	}
	oldLocal, oldHome := serveGuardLocal, serveGuardHome
	serveGuardLocal, serveGuardHome = func() deployguard.Local { return quietHost() }, home
	t.Cleanup(func() {
		dispatchCloneRef, dispatchRun, serveGuardLocal, serveGuardHome = clone, execute, oldLocal, oldHome
	})
	code := make(chan int, 1)
	go func() {
		code <- runV15([]string{"serve", "--team", "--insecure", "--addr", "127.0.0.1:0", "--workdir", t.TempDir(), "--data-dir", home})
	}()
	var row store.IntakeRow
	deadline := time.Now().Add(120 * time.Second)
	exited := false
	for time.Now().Before(deadline) && !exited {
		select {
		case c := <-code:
			t.Errorf("serve stopped on its own with exit %d", c)
			exited = true
			continue
		case <-time.After(200 * time.Millisecond):
		}
		st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
		if err != nil {
			continue
		}
		rows, err := st.IntakeList(context.Background(), 5)
		_ = st.Close()
		if err == nil && len(rows) == 1 && (rows[0].State == store.IntakeDone || rows[0].State == store.IntakeFailed) {
			row = rows[0]
			break
		}
	}
	if !exited {
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		select {
		case <-code:
		case <-time.After(30 * time.Second):
			t.Fatal("serve did not stop")
		}
	}
	return row
}

func TestWire01TeamPlacesOnNode(t *testing.T) {
	repo, home, sha, node := wire01Fixture(t, true, "")
	row := wire01Serve(t, repo, home)
	if row.PipelineID == "" {
		t.Fatalf("serve did not finish the pipeline: %+v", row)
	}
	got := node.ran()
	if len(got) != 1 || got[0].Checkout.Revision != sha {
		t.Fatalf("the ordinary job did not run on the enrolled node: %d leases, row %+v", len(got), row)
	}
	// the invocation's controls travel in the job: serve's default --container and --timeout reach the node
	d, err := time.ParseDuration(got[0].Job.Timeout)
	if j := got[0].Job; j.Isolation != "container" || err != nil || d <= 0 || d > 600*time.Second {
		t.Fatalf("the node's job does not carry serve's --container and --timeout: isolation %q timeout %q", j.Isolation, j.Timeout)
	}
}

func TestWire01UnplaceableFailsWithQueueCap(t *testing.T) {
	repo, home, _, _ := wire01Fixture(t, false, `"3s"`)
	start := time.Now()
	row := wire01Serve(t, repo, home)
	if row.PipelineID == "" {
		t.Fatalf("an unplaceable job waited instead of failing: %+v", row)
	}
	if time.Since(start) > 100*time.Second {
		t.Fatalf("the unplaceable job did not fail within the cap: %s", time.Since(start))
	}
	if row.State != store.IntakeDone || row.Result != "exit=2" {
		t.Fatalf("the job did not fail: %+v", row)
	}
	dump, err := exec.Command("sqlite3", filepath.Join(home, "state.db"), ".dump").CombinedOutput()
	if err != nil {
		t.Skipf("no sqlite3 to read the failure code: %v", err)
	}
	if !strings.Contains(string(dump), "E693") && !strings.Contains(string(dump), "E618") {
		t.Fatalf("the job failed, but not with E618 or E693")
	}
}
