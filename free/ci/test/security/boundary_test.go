//go:build security

package security

// Purpose: the production boundary and the telemetry promise.
//   I15 (TM-41): a deploy host is never selected without the operator's override; an override never
//   admits untrusted work, release or deploy secrets or isolation below vm, and every placement on an
//   overridden host leaves an audit row inside the admission transaction; the coordinator host needs
//   vm for team mode and for other people's work.
//   X15 (TM-36): no request reaches a telemetry endpoint from the plugin or the agent in any mode.
//   The telemetry fake of main_test.go is both the ping_api endpoint and the process proxy; this test
//   drives the code that records scheduling decisions with the default sink and asserts it stayed quiet.
// Inputs:  the dispatcher rig; trust.Admissible over the enumerated classes.
// Outputs: pass or fail.
// Constraints: the default sink is the production telemetry sink (internal/schedtel registers it);
//          the suite as a whole fails if the fake ever records a request (main_test.go).

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	_ "github.com/nself-org/plugins/free/ci/internal/schedtel" // registers the telemetry sink the dispatcher uses by default
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// deployHostDecl is the declaration of a LAN node the inventory matched as a deploy host.
func deployHostDecl(isolation model.Isolation, override bool) trust.Declaration {
	d := lanRunner("deploy1", 4).Decl
	d.Isolation = isolation
	d.DeployHost = trust.DeployHost{Matched: true, Override: override}
	return d
}

// TestDeployHostNeverSelectedWithoutOverride (I15): over every trust class, isolation level and
// secret class the product defines, the rule's answer is the one the invariant states.
func TestDeployHostNeverSelectedWithoutOverride(t *testing.T) {
	eff := repoPolicy(t, "private", "owner")
	checked := 0
	for _, class := range model.EnumValues("TrustClass") {
		for _, iso := range model.EnumValues("Isolation") {
			for _, secret := range append([]string{"none"}, model.EnumValues("SecretClass")...) {
				for _, override := range []bool{false, true} {
					decl := deployHostDecl(model.Isolation(iso), override)
					job := trust.JobFacts{Trust: model.TrustClass(class), Network: "restricted", SecretClasses: []model.SecretClass{model.SecretClass(secret)}}
					reasons := trust.Admissible(job, decl, eff)
					boundary := false
					for _, r := range reasons {
						boundary = boundary || r == trust.BoundaryDeployHost || r == trust.BoundaryOverrideMinimum
					}
					own := class == "owner" || class == "internal"
					releaseOrDeploy := secret == "release" || secret == "deploy" || secret != "none" && secret != "project" && secret != "environment" && secret != "team"
					allowed := override && own && trust.RankIsolation(model.Isolation(iso)) >= trust.RankIsolation("vm") && !releaseOrDeploy
					if allowed == boundary {
						t.Fatalf("class=%s isolation=%s secret=%s override=%v: boundary refusal=%v, want %v (%v)", class, iso, secret, override, boundary, !allowed, reasons)
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("nothing was checked")
	}
}

// TestDeployHostPlacementThroughDispatcher: the dispatcher keeps work off a deploy host, places an
// owner job on an overridden one only with its audit row, and refuses an untrusted job there.
func TestDeployHostPlacementThroughDispatcher(t *testing.T) {
	plain := lanRunner("lan-ok", 1)
	deploy := lanRunner("deploy1", 4)
	deploy.Decl.DeployHost = trust.DeployHost{Matched: true}

	// 1. Without an override the deploy host is never chosen, even though it has the most room.
	r := newRig(t, repoPolicy(t, "private", "owner"), deploy, plain)
	for i := 0; i < 3; i++ {
		id, a, err := r.submit(newJob("work"+string(rune('a'+i)), "owner", "lan"), "owner", settle)
		if i == 0 {
			if got := runnerOf(t, a, err); got != "lan-ok" {
				t.Fatalf("job placed on %q, want lan-ok", got)
			}
			continue
		}
		waitsBecause(t, r, id, a, err, string(trust.BoundaryDeployHost)) // the ordinary node has one slot: the rest wait, they do not spill to the deploy host
		if d, ok := r.sink.last(id); ok && d.Placement != nil && d.Placement.RunnerID == "deploy1" {
			t.Fatal("work spilled onto a deploy host without an override")
		}
	}
	// 2. With the override, an owner job on vm is placed there and the audit row exists.
	deploy.Decl.DeployHost.Override = true
	r = newRig(t, repoPolicy(t, "private", "owner"), deploy)
	id, a, err := r.submit(newJob("deploy-owner", "owner", "lan"), "owner", 2*time.Second)
	if got := runnerOf(t, a, err); got != "deploy1" {
		t.Fatalf("overridden deploy host not used by an owner job: %q", got)
	}
	rows, err := r.st.AuditEntries(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		found = found || row.Action == dispatch.DeployHostAuditAction && row.Target == id && strings.Contains(row.Detail, "runner=deploy1")
	}
	if !found {
		t.Fatalf("no %s audit row for %s among %d rows", dispatch.DeployHostAuditAction, id, len(rows))
	}
	// 3. Even overridden, untrusted work and release secrets are refused there.
	r = newRig(t, repoPolicy(t, "public", "untrusted"), deploy)
	id2, a2, err2 := r.submit(newJob("deploy-fork", "untrusted", "restricted"), "untrusted", settle)
	waitsBecause(t, r, id2, a2, err2, "boundary.override_minimum")
	r = newRig(t, repoPolicy(t, "private", "owner"), deploy)
	id3, a3, err3 := r.submit(newJob("deploy-release", "owner", "lan", "release"), "owner", settle)
	waitsBecause(t, r, id3, a3, err3, "boundary.override_minimum")
}

// TestCoordinatorHostFloor (D28): a team coordinator host below vm takes nobody's work, and a
// personal coordinator host below vm takes only its operator's.
func TestCoordinatorHostFloor(t *testing.T) {
	eff := repoPolicy(t, "private", "owner")
	host := lanRunner("coord", 4).Decl
	host.Isolation, host.CoordinatorHost = "container", true
	host.CoordinatorMode = trust.Team
	if got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, host, eff); !hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("team coordinator host below vm took an owner job: %v", got)
	}
	host.CoordinatorMode = trust.Personal
	if got := trust.Admissible(trust.JobFacts{Trust: "collaborator", Network: "internet"}, host, eff); !hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("personal coordinator host below vm took a collaborator's job: %v", got)
	}
	if got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, host, eff); hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("control: a personal coordinator host refused its own operator's job: %v", got)
	}
	host.Isolation = "vm"
	host.CoordinatorMode = trust.Team
	if got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, host, eff); hasTrustReason(got, trust.BoundaryCoordinatorHost) {
		t.Errorf("control: a vm coordinator host was refused: %v", got)
	}
}

// TestNoRequestReachesTelemetry (X15, TM-36): scheduling decisions are recorded through the default
// telemetry sink while the endpoint variables and the process proxy all point at the telemetry
// fake; the fake sees nothing. The fake's own count is also asserted at the end of the suite.
func TestNoRequestReachesTelemetry(t *testing.T) {
	st := openStore(t)
	registerKinds(t, lanRunner("lan1", 4), hostedRunner("hosted1"))
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", Transport: newFabricTransport(), Poll: 10 * time.Millisecond,
		World: worldOf(st, repoPolicy(t, "private", "owner"), lanRunner("lan1", 4), hostedRunner("hosted1"))}) // Sink nil: the production default
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	for i := 0; i < 5; i++ {
		id := queueAttempt(t, st, "tele"+string(rune('a'+i)), "project", "job", "owner")
		j := newJob(id, "owner", "lan")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		a, err := d.Submit(ctx, dispatch.Job{AttemptID: id, Sched: j, Revision: strings.Repeat("a", 40)})
		cancel()
		if err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
		pass(t, st, a.AttemptID)
	}
	time.Sleep(200 * time.Millisecond) // a background sender, if one existed, would have fired by now
	if seen := telemetryRequests(); len(seen) != 0 {
		t.Fatalf("the plugin sent %d request(s) to the telemetry endpoint or proxy: %v", len(seen), seen)
	}
}

// TestNoTelemetryClientInTheProduct (X15): the agent cannot send what it has no client for. No package of
// the agent imports net/http, and no string literal of the product names a telemetry or analytics
// service or the ping_api. This is the static half of the zero-request promise; the runtime half is the
// fake of main_test.go, which cannot see a client that ignores the proxy variables and an endpoint it
// was never told.
func TestNoTelemetryClientInTheProduct(t *testing.T) {
	root := filepath.Join("..", "..")
	banned := regexp.MustCompile(`(?i)ping\.nself\.org|ping_api|posthog|plausible|segment\.io|google-analytics|amplitude|datadog|sentry\.io|mixpanel`)
	fset := token.NewFileSet()
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == "test") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		checked++
		slash := filepath.ToSlash(path)
		// internal/agent/install downloads the agent release the operator named (a one-way fetch of a binary
		// by checksum); every other package of the agent runs jobs and speaks only the carrier protocol.
		inAgent := (strings.Contains(slash, "/internal/agent/") || strings.Contains(slash, "/cmd/agent/")) && !strings.Contains(slash, "/internal/agent/install/")
		for _, imp := range file.Imports {
			if inAgent && imp.Path.Value == `"net/http"` {
				t.Errorf("%s: the agent imports net/http", slash)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && banned.MatchString(lit.Value) {
				t.Errorf("%s: string literal %s names a telemetry endpoint", slash, lit.Value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Fatalf("only %d product files were read: the walk is not reaching the module", checked)
	}
}

// decisionPackages are the packages that decide, record or carry CI data without needing a network:
// admission and scheduling, trust, the store, evidence, authorization, roles, audit, secrets, redaction,
// hygiene and scheduling telemetry, plus the agent's job-running packages. None of them may import an HTTP,
// RPC or mail client, or dial, listen or resolve a name. Packages that exist to talk (the coordinator,
// providers, the user client, the proxy, node install) are outside this list on purpose: a new client there
// is a review question for its own Ticket, not a tripwire for this suite.
var decisionPackages = []string{"internal/dispatch", "internal/sched", "internal/schedtel", "internal/schedpolicy", "internal/trust", "internal/store", "internal/attest",
	"internal/authz", "internal/roles", "internal/audit", "internal/secrets", "internal/redact", "internal/hygiene", "internal/invariants", "internal/policy",
	"internal/model", "internal/evidence", "internal/events", "internal/history", "internal/lease", "internal/agent/session", "internal/agent/slots",
	"internal/agent/checkout", "internal/agent/nodekey", "internal/agent/agekey", "internal/agent/contention", "internal/agent/verify"}

// TestDecisionPackagesHaveNoNetworkCode (X15): the packages that handle CI decisions and data have no
// network client to send them with. The test follows import aliases (import n "net") and counts the
// calls that dial, listen or resolve. It sees Go source only: a raw syscall socket or an exec of curl would
// pass, and so would a package outside the list; the runtime fake of main_test.go and the review of a
// Ticket that adds a client cover those.
func TestDecisionPackagesHaveNoNetworkCode(t *testing.T) {
	fset := token.NewFileSet()
	files := 0
	for _, pkg := range decisionPackages {
		dir := filepath.Join("..", "..", filepath.FromSlash(pkg))
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("%s: %v (a package left the list's tree: update decisionPackages)", pkg, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			files++
			netNames := map[string]bool{}
			for _, imp := range file.Imports {
				switch p := strings.Trim(imp.Path.Value, `"`); p {
				case "net/http", "net/rpc", "net/smtp", "net/http/httputil":
					t.Errorf("%s imports %s", filepath.ToSlash(path), p)
				case "net":
					name := "net"
					if imp.Name != nil {
						name = imp.Name.Name
					}
					netNames[name] = true
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && netNames[id.Name] {
						switch name := sel.Sel.Name; {
						case strings.HasPrefix(name, "Dial"), strings.HasPrefix(name, "Lookup"), name == "Listen", name == "ListenPacket", name == "ListenTCP", name == "ListenUDP":
							t.Errorf("%s calls net.%s", filepath.ToSlash(path), name)
						}
					}
				}
				return true
			})
		}
	}
	if files < 200 {
		t.Fatalf("only %d files of the decision packages were read: the walk is not reaching the module", files)
	}
}
