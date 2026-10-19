package deployguard

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// A never-list host of exactly 43 characters is a host, not a fingerprint.
func TestNeverListHostOfFingerprintLengthStaysAHost(t *testing.T) {
	host := strings.Repeat("a", 30) + ".example.test"
	if len(host) != 43 {
		t.Fatalf("fixture length %d", len(host))
	}
	for _, v := range []string{host, "ci@" + host[:len(host)-8] + ".test"} {
		e, ok, err := neverEntry(v)
		if err != nil || !ok || e.Kind != KindHost {
			t.Errorf("%q: %+v %v %v", v, e, ok, err)
		}
	}
	s := NewSet([]Entry{func() Entry { e, _, _ := neverEntry(host); return e }()})
	if !s.MatchHost(host).Matched || !s.MatchHost("ci@"+strings.ToUpper(host)+":22").Matched {
		t.Fatal("the 43-character host does not match")
	}
	if e, ok, _ := neverEntry("sha256:" + FixtureFingerprint[7:]); !ok || e.Kind != KindFingerprint {
		t.Fatalf("a prefixed fingerprint: %+v", e)
	}
	if NormalizeFingerprint(host) != "" || NormalizeFingerprint(strings.Repeat("a", 42)+"!") != "" {
		t.Fatal("a 43-character string outside the base64 alphabet was accepted as a fingerprint")
	}
}

func TestStoredSSHHostnameIsNotQuiet(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	c := r.capability("n", func(c *model.Capability) {
		c.Identity.SSH = &model.CapabilitySSH{Alias: "ci-box", Hostname: "box.example.test", Port: 22, User: "ci"}
	})
	id := IdentityOf(*c)
	if id.Dest != "ci-box" || id.Hostname != "box.example.test" || len(id.Names) != 2 {
		t.Fatalf("identity %+v", id)
	}
	r.ssh["ci-box"] = remote.Resolved{Hostname: "box.example.test"}
	if f, _ := r.g.Resolver.Facts(ctx, id); !f.Partial {
		t.Fatal("a stored hostname with no address must be Partial")
	}
	r.lookup["box.example.test"] = nil
	if f, w := r.g.Resolver.Facts(ctx, id); f.Partial || len(w) != 0 {
		t.Fatalf("resolved: %+v %v", f, w)
	}
}

func TestCheckAddRefusesAnUnmatchedNodeWhenSSHCannotResolveIt(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	c := r.capability("pb", ssh("prodbox.example.test"))
	_, warn, err := r.g.CheckAdd(ctx, AddRequest{NodeID: "pb", Dest: "ci@prodbox.example.test", Capability: c})
	if ReasonOf(err) != ReasonNoIdentity || len(warn) == 0 || c.DeployHost.Matched || !strings.Contains(err.Error(), "node was not added") {
		t.Fatalf("%v %v %+v", err, warn, c.DeployHost)
	}
	// ssh -G answers but DNS does not (a name only a jump host resolves): added, with a warning.
	r.ssh["prodbox.example.test"] = remote.Resolved{Hostname: "prodbox.example.test"}
	if _, warn, err = r.g.CheckAdd(ctx, AddRequest{NodeID: "pb", Dest: "ci@prodbox.example.test", Capability: c}); err != nil || len(warn) == 0 {
		t.Fatalf("DNS-only gap: %v %v", err, warn)
	}
	// With everything answering, no warning.
	r.lookup["prodbox.example.test"] = nil
	if _, warn, err = r.g.CheckAdd(ctx, AddRequest{NodeID: "pb", Dest: "ci@prodbox.example.test", Capability: c}); err != nil || len(warn) != 0 {
		t.Fatalf("%v %v", err, warn)
	}
}

func TestParseTargetsRefusesNullDocuments(t *testing.T) {
	for name, doc := range map[string]string{
		"null data":    `{"command":"deploy targets","data":null}`,
		"null targets": `{"command":"deploy targets","data":{"targets":null}}`,
		"no data":      `{"command":"deploy targets"}`,
		"data string":  `{"command":"deploy targets","data":"x"}`,
	} {
		if _, err := parseTargets([]byte(doc)); ReasonOf(err) != ReasonInventory {
			t.Errorf("%s: %v", name, err)
		}
	}
	if es, err := parseTargets([]byte(`{"command":"deploy targets","data":{"targets":[]}}`)); err != nil || len(es) != 0 {
		t.Fatalf("a real empty list: %v %v", es, err)
	}
}

func TestEmptyReadNeverReplacesACachedInventoryButForgetDropsIt(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	r.run.HostsByRoot[r.rootA] = []string{}
	err := r.g.RefreshProject(ctx, r.rootA)
	if err == nil || !strings.Contains(err.Error(), "keeping them") {
		t.Fatalf("empty over cache: %v", err)
	}
	if s, _ := r.g.Set(ctx); !s.MatchHost(FixtureHostV4).Matched {
		t.Fatal("the cache shrank to empty")
	}
	if err = r.g.ForgetProject(ctx, r.rootA); err != nil {
		t.Fatal(err)
	}
	if s, _ := r.g.Set(ctx); s.MatchHost(FixtureHostV4).Matched {
		t.Fatal("forget kept the entries")
	}
	if roots, _ := r.st.ExclusionProjects(ctx); len(roots) != 0 || len(r.g.Warnings(ctx)) != 0 {
		t.Fatalf("roots %v warnings %v", roots, r.g.Warnings(ctx))
	}
	if r.auditCount("exclusion.forget") != 1 {
		t.Fatal("forget not audited once")
	}
	// A project that never had entries may read as empty.
	if err = r.g.RefreshProject(ctx, r.rootA); err != nil {
		t.Fatalf("empty over empty: %v", err)
	}
	if err = r.g.ForgetProject(ctx, ""); err == nil {
		t.Fatal("empty root forgotten")
	}
}

func TestShrinkIsAudited(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootB)
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if r.auditCount("exclusion.shrink") != 0 {
		t.Fatal("first read audited as a shrink")
	}
	r.run.HostsByRoot[r.rootB] = []string{FixtureHostName}
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if r.auditCount("exclusion.shrink") != 1 {
		t.Fatalf("shrink rows = %d", r.auditCount("exclusion.shrink"))
	}
	if err := r.g.Refresh(ctx); err != nil || r.auditCount("exclusion.shrink") != 1 {
		t.Fatalf("an unchanged read audited: %v %d", err, r.auditCount("exclusion.shrink"))
	}
}

func TestUnreadableProdEnvRefusesTheStart(t *testing.T) {
	dir := "/srv/stack"
	pathWith(t, true)
	unreadable := Local{
		Hostname: func() (string, error) { return "h", nil },
		ReadFile: func(p string) ([]byte, error) {
			if strings.HasSuffix(p, ".env") {
				return nil, os.ErrPermission
			}
			return nil, os.ErrNotExist
		},
		Run: dockerOut(dir, 0),
	}
	_, err := unreadable.Check(context.Background(), Set{}, "serve")
	if ReasonOf(err) != ReasonMarker || !strings.Contains(err.Error(), "cannot verify") {
		t.Fatalf("%v", err)
	}
	// An absent .env and .env.prod is a calm project; a nil reader cannot judge and does not refuse.
	absent := unreadable
	absent.ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if _, err = absent.Check(context.Background(), Set{}, "serve"); err != nil {
		t.Fatal(err)
	}
	if ok, err := (Local{}).prodEnv(dir); ok || err != nil {
		t.Fatalf("nil reader: %v %v", ok, err)
	}
}

func TestRunnerCapsItsOutput(t *testing.T) {
	out, exit, err := FunnelRun(context.Background(), "", "sh", "-c", "head -c 6000000 /dev/zero | tr '\\0' a")
	if err != nil || exit != 0 || len(out) != maxOutput {
		t.Fatalf("len %d exit %d err %v", len(out), exit, err)
	}
	out, _, _ = FunnelRun(context.Background(), "", "sh", "-c", "printf short")
	if string(out) != "short" {
		t.Fatalf("%q", out)
	}
}

func TestFixtureSSHConfigUsesRealKeywords(t *testing.T) {
	cfg := FixtureSSHConfig()
	for _, kw := range []string{"\nHost ", "\n  HostName ", "\n  ProxyJump ", "\n  HostKeyAlias "} {
		if !strings.Contains("\n"+cfg, kw) {
			t.Errorf("missing %q in %q", kw, cfg)
		}
	}
	if strings.Contains(cfg, "Fixture") {
		t.Errorf("a search-replace leaked into the config: %q", cfg)
	}
}

func TestBareFingerprintInTheNeverListIsAnErrorNotASilentHost(t *testing.T) {
	bare := FixtureFingerprint[7:] // 43 base64 characters, no dot
	if _, ok, err := neverEntry(bare); err == nil || ok || ReasonOf(err) != ReasonInventory || !strings.Contains(err.Error(), "SHA256:") {
		t.Fatalf("bare fingerprint: ok=%v err=%v", ok, err)
	}
	if e, ok, err := neverEntry("SHA256:" + bare); err != nil || !ok || e.Kind != KindFingerprint {
		t.Fatalf("prefixed: %+v %v %v", e, ok, err)
	}
	if e, ok, err := neverEntry("host-" + strings.Repeat("b", 24) + ".example.test"); err != nil || !ok || e.Kind != KindHost {
		t.Fatalf("a dotted host: %+v %v %v", e, ok, err)
	}
}

func TestAnUncheckedAddIsSaidOutLoud(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_, warn, err := r.g.CheckAdd(ctx, AddRequest{NodeID: "x", Dest: "ci@" + FixtureOther})
	if err != nil || len(warn) != 1 || !strings.Contains(warn[0], "nothing was checked") {
		t.Fatalf("%v %v", warn, err)
	}
	// With any entry known the warning is gone.
	r.writeConfig("ci:\n  never_hosts: [198.51.100.9]\n")
	if _, warn, err = r.g.CheckAdd(ctx, AddRequest{NodeID: "x", Dest: "ci@" + FixtureOther}); err != nil || len(warn) != 0 {
		t.Fatalf("%v %v", warn, err)
	}
	// So is a recorded project root with no entries (an empty inventory is still an inventory).
	r2 := newRig(t)
	_ = r2.g.Seen(ctx, r2.rootA)
	r2.run.HostsByRoot[r2.rootA] = []string{}
	if _, warn, err = r2.g.CheckAdd(ctx, AddRequest{NodeID: "x", Dest: "ci@" + FixtureOther}); err != nil || len(warn) != 0 {
		t.Fatalf("known root: %v %v", warn, err)
	}
}

func TestSwappingOneEntryForAnotherIsAShrink(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	_ = r.g.Seen(ctx, r.rootA)
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	r.run.HostsByRoot[r.rootA] = []string{FixtureOther} // same count of hosts, a different one
	if err := r.g.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if r.auditCount("exclusion.shrink") != 1 {
		t.Fatalf("shrink rows = %d", r.auditCount("exclusion.shrink"))
	}
}
