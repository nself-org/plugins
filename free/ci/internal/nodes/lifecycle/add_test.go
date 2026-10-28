package lifecycle

import (
	"context"
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestAddAuthorizesCurrentProjectOnly(t *testing.T) {
	k := newKit(t)
	r := k.addNode("sim1")
	lc := r.Node.Capability.Lifecycle
	if len(lc.AuthorizedProjects) != 1 || lc.AuthorizedProjects[0] != "proj-1" || lc.Discovered || lc.RevokedAt != nil {
		t.Fatalf("lifecycle = %+v", lc)
	}
	acc := r.Node.Capability.Trust.Accepts.Value
	if acc == nil || len(*acc) != 2 || (*acc)[0] != "owner" || (*acc)[1] != "internal" {
		t.Fatalf("accepts = %v", acc)
	}
	sc := r.Node.Capability.Trust.SecretClasses.Value
	if sc == nil || len(*sc) != 1 || (*sc)[0] != "none" {
		t.Fatalf("secret classes = %v", sc)
	}
	if r.Node.Capability.Platform.OS.Value == nil || r.AgentVersion != "1.5.0" {
		t.Fatalf("snapshot missing: os=%v agent=%q", r.Node.Capability.Platform.OS.Value, r.AgentVersion)
	}
	pinned, err := k.d.pins().Lookup(pinAlias("sim1"))
	if err != nil || len(pinned) != 1 {
		t.Fatalf("pin: %v %v", pinned, err)
	}
	if n := k.auditCount("node.add"); n != 1 || k.auditCount("") != 1 {
		t.Fatalf("audit rows: add=%d all=%d, want exactly one", n, k.auditCount(""))
	}
	if len(k.copies) != 1 {
		t.Fatalf("agent copies = %d", len(k.copies))
	}
}

func TestNodeReportedSecretClassesAreIgnored(t *testing.T) {
	k := newKit(t)
	k.mutate = func(c *model.Capability) {
		rel := []model.SecretClass{"release"}
		all := []model.TrustClass{"owner", "collaborator", "internal"}
		c.Trust.SecretClasses.Value = &rel
		c.Trust.Accepts.Value = &all
	}
	r := k.addNode("sim2")
	sc := r.Node.Capability.Trust.SecretClasses.Value
	if sc == nil || len(*sc) != 1 || (*sc)[0] != "none" {
		t.Fatalf("X5: node-declared secret_classes reached the store: %v", sc)
	}
	acc := r.Node.Capability.Trust.Accepts.Value
	if acc == nil || len(*acc) != 2 {
		t.Fatalf("node-declared accepts reached the store: %v", acc)
	}
}

func TestAdminTrustFlagsApply(t *testing.T) {
	k := newKit(t)
	o := AddOptions{Dest: "ci@sim3", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)},
		Trust: TrustFlags{Accepts: []string{"owner"}, SecretClasses: []string{"project"}, PrivacyZone: "local-only", Ownership: "team", AcceptsSet: true}}
	r, err := k.d.Add(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	c := r.Node.Capability
	if c.Trust.PrivacyZone.Value == nil || string(*c.Trust.PrivacyZone.Value) != "local-only" || c.Identity.Ownership.Value == nil || *c.Identity.Ownership.Value != "team" {
		t.Fatalf("flags not applied: zone=%v own=%v", c.Trust.PrivacyZone.Value, c.Identity.Ownership.Value)
	}
}

func TestNoTTYWithoutHostKeyRefusesAndPinsNothing(t *testing.T) {
	k := newKit(t)
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@sim4", Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	errCode(t, err, CodeHostKey)
	if e := err.(*Error); e.Reason != ReasonNoTTY {
		t.Fatalf("reason = %s", e.Reason)
	}
	if p, _ := k.d.pins().Lookup(pinAlias("sim4")); len(p) != 0 {
		t.Fatalf("pinned without confirmation: %v", p)
	}
	if _, err = k.d.Registry.Get(context.Background(), "sim4"); CodeOf(err) != CodeNotFound {
		t.Fatalf("node registered: %v", err)
	}
	if len(k.runs) != 0 || k.auditCount("") != 0 {
		t.Fatalf("ssh ran or audit written before confirmation: runs=%d", len(k.runs))
	}
}

func TestInteractiveConfirmation(t *testing.T) {
	for _, tc := range []struct {
		in   string
		ok   bool
		name string
	}{{"yes\n", true, "yes"}, {"no\n", false, "no"}, {"\n", false, "empty"}, {"y\n", false, "y-is-not-yes"}} {
		t.Run(tc.name, func(t *testing.T) {
			k := newKit(t)
			k.d.TTY, k.d.Stdin = true, strings.NewReader(tc.in)
			_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@sim5", Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
			if tc.ok != (err == nil) {
				t.Fatalf("err = %v", err)
			}
			if !tc.ok {
				errCode(t, err, CodeHostKey)
				if p, _ := k.d.pins().Lookup(pinAlias("sim5")); len(p) != 0 {
					t.Fatal("declined add left a pin")
				}
			}
			if !strings.Contains(k.out.String(), fpA) {
				t.Fatalf("fingerprint not shown: %q", k.out.String())
			}
		})
	}
}

func TestHostKeyFlagMustMatch(t *testing.T) {
	k := newKit(t)
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@sim6", HostKey: fpB, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	errCode(t, err, CodeHostKey)
	if p, _ := k.d.pins().Lookup(pinAlias("sim6")); len(p) != 0 {
		t.Fatal("a wrong --host-key pinned something")
	}
}

func TestFailedAddRollsBackPinAndRegistersNothing(t *testing.T) {
	k := newKit(t)
	k.failRun["uname"] = errBoom
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@sim7", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	errCode(t, err, CodeNoRoute)
	if p, _ := k.d.pins().Lookup(pinAlias("sim7")); len(p) != 0 {
		t.Fatal("failed add kept its pin")
	}
	if k.auditCount("") != 0 {
		t.Fatal("failed add wrote an audit row")
	}
}

func TestAddRefusals(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	elfOK := LocalFile{Path: elfFixture(t, elf.EM_X86_64)}
	for name, o := range map[string]AddOptions{
		"port":         {Dest: "ci@host:2222", HostKey: fpA, Source: elfOK, Trust: DefaultTrust()},
		"legacy path":  {Dest: "ci@host:/srv/x", HostKey: fpA, Source: elfOK, Trust: DefaultTrust()},
		"bad accepts":  {Dest: "ci@host", HostKey: fpA, Source: elfOK, Trust: TrustFlags{Accepts: []string{"root"}, SecretClasses: []string{"none"}, Ownership: "operator"}},
		"bad owner":    {Dest: "ci@host", HostKey: fpA, Source: elfOK, Trust: TrustFlags{Accepts: []string{"owner"}, SecretClasses: []string{"none"}, Ownership: "me"}},
		"wrong arch":   {Dest: "ci@host", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_AARCH64)}, Trust: DefaultTrust()},
		"not a binary": {Dest: "ci@host", HostKey: fpA, Source: LocalFile{Path: filepath.Join(t.TempDir(), "missing")}, Trust: DefaultTrust()},
	} {
		if _, err := k.d.Add(ctx, o); CodeOf(err) != CodeInvalid {
			t.Errorf("%s: err = %v, want E651", name, err)
		}
	}
	k.addNode("dup")
	if _, err := k.d.Add(ctx, AddOptions{Dest: "ci@dup", HostKey: fpA, Trust: DefaultTrust()}); CodeOf(err) != CodeInvalid {
		t.Errorf("duplicate add: %v", err)
	}
	k.d.Project = ""
	if _, err := k.d.Add(ctx, AddOptions{Dest: "ci@other", HostKey: fpA, Trust: DefaultTrust()}); CodeOf(err) != CodeInvalid {
		t.Errorf("no project: %v", err)
	}
}

func TestNoSeparationAcceptsOwnerOnly(t *testing.T) {
	k := newKit(t)
	k.d.Separation = fixedSeparation{SeparationFacts{Reason: "probe_unavailable"}}
	r := k.addNode("nosep")
	c := r.Node.Capability
	if c.Separation.UIDSeparation.Value == nil || *c.Separation.UIDSeparation.Value {
		t.Fatalf("uid_separation = %v, want false", c.Separation.UIDSeparation.Value)
	}
	acc := c.Trust.Accepts.Value
	if acc == nil || len(*acc) != 1 || (*acc)[0] != "owner" {
		t.Fatalf("accepts = %v, want [owner]", acc)
	}
	if c.SecretsEligible {
		t.Fatal("secrets eligible without separation")
	}
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@nosep2", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)},
		Trust: TrustFlags{Accepts: []string{"owner", "internal"}, SecretClasses: []string{"none"}, Ownership: "operator", AcceptsSet: true}})
	errCode(t, err, CodeInvalid)
}

func TestAgentSeparationProbe(t *testing.T) {
	ctx := context.Background()
	runWith := func(out string, err error) func(context.Context, ...string) (string, error) {
		return func(context.Context, ...string) (string, error) { return out, err }
	}
	linux := Platform{"linux", "amd64"}
	for name, tc := range map[string]struct {
		out  string
		err  error
		p    Platform
		want bool
	}{
		"available":      {`{"available":true,"reason":"userns.ok"}`, nil, linux, true},
		"unavailable":    {`{"available":false,"reason":"userns.denied"}`, nil, linux, false},
		"garbage":        {"not json", nil, linux, false},
		"ssh error":      {"", errBoom, linux, false},
		"darwin":         {`{"available":true}`, nil, Platform{"darwin", "arm64"}, false},
		"hostile reason": {`{"available":false,"reason":"x y; rm -rf"}`, nil, linux, false},
	} {
		got := AgentSeparation{}.Detect(ctx, runWith(tc.out, tc.err), tc.p)
		if got.UIDSeparation != tc.want {
			t.Errorf("%s: separated=%v want %v", name, got.UIDSeparation, tc.want)
		}
		if strings.ContainsAny(got.Reason, " ;") {
			t.Errorf("%s: reason %q is not a key", name, got.Reason)
		}
	}
}

func TestCheckBinaryAndPlatform(t *testing.T) {
	if _, err := ParsePlatform("FreeBSD amd64"); err == nil {
		t.Error("freebsd accepted")
	}
	if _, err := ParsePlatform("Linux riscv64"); err == nil {
		t.Error("riscv accepted")
	}
	if p, err := ParsePlatform("Darwin arm64"); err != nil || p != (Platform{"darwin", "arm64"}) {
		t.Errorf("darwin: %v %v", p, err)
	}
	dir := t.TempDir()
	text := filepath.Join(dir, "script.sh")
	_ = os.WriteFile(text, []byte("#!/bin/sh\necho hi\n"), 0o755)
	empty := filepath.Join(dir, "empty")
	_ = os.WriteFile(empty, nil, 0o755)
	for _, p := range []string{text, empty, dir} {
		if err := CheckBinary(p, Platform{"linux", "amd64"}); CodeOf(err) != CodeInvalid {
			t.Errorf("%s: %v", p, err)
		}
	}
	if err := CheckBinary(elfFixture(t, elf.EM_X86_64), Platform{"darwin", "amd64"}); CodeOf(err) != CodeInvalid {
		t.Errorf("elf on darwin node: %v", err)
	}
}

func TestNoAgentFlagNeedsInstalledAgent(t *testing.T) {
	k := newKit(t)
	k.failRun["test"] = errBoom
	_, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@noagent", HostKey: fpA, Trust: DefaultTrust()})
	errCode(t, err, CodeInvalid)
	if !strings.Contains(err.Error(), "--agent-binary") {
		t.Fatalf("message does not name the fix: %v", err)
	}
}

func TestNodeIDAndHostKeyHelpers(t *testing.T) {
	spec, _ := remote.ParseHostSpec("ci@Build-Box.local")
	id, name, err := nodeID(AddOptions{}, spec)
	if err != nil || id != "build-box.local" || name != "Build-Box.local" {
		t.Fatalf("id=%q name=%q err=%v", id, name, err)
	}
	if _, _, err = nodeID(AddOptions{Name: "bad name!"}, spec); err == nil {
		t.Fatal("bad name accepted")
	}
	if kl := keyLine(hostKey("ssh-ed25519", fpA, "AAAA")); kl != "ssh-ed25519 AAAA" || keyLine(keyA) == "" {
		t.Fatalf("keyLine = %q", kl)
	}
}

// D-1212: a read-back failure after the node was committed still reports the node that was written.
func TestAddReportsTheProbedNodeWhenTheReadBackFails(t *testing.T) {
	k := newKit(t)
	blind, err := store.Open(filepath.Join(t.TempDir(), "blind.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = blind.Close() })
	k.d.Registry = registry.New(blind) // reads an empty store; the writes still go to k.d.Store
	r, err := k.d.Add(context.Background(), AddOptions{Dest: "ci@rb1", HostKey: fpA, Source: LocalFile{Path: elfFixture(t, elf.EM_X86_64)}, Trust: DefaultTrust()})
	if err != nil {
		t.Fatal(err)
	}
	v := ViewOf(r.Node)
	if v.ID != "rb1" || v.State != "online" || len(v.AuthorizedProjects) != 1 || v.AuthorizedProjects[0] != "proj-1" || r.Node.Capability.Identity.ID != "rb1" {
		t.Fatalf("the add result is an empty node: %+v", v)
	}
	if _, gerr := k.st.NodeGet(context.Background(), "rb1"); gerr != nil {
		t.Fatalf("the node was not committed: %v", gerr)
	}
}
