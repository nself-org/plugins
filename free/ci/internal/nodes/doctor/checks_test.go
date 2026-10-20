package doctor

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func fact[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "self", ObservedAt: t0, Confidence: "known"}
}

// capFixture is the valid laptop capability from the model fixtures.
func capFixture(t testing.TB) model.Capability {
	t.Helper()
	b, err := os.ReadFile("../../model/testdata/capability/valid/laptop.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var c model.Capability
	if err = json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// sshNode and agentNode are healthy nodes of the two remote transports.
func sshNode(t testing.TB, id string) Node {
	c := capFixture(t)
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: id + ".lan.test", Port: 22, User: "ci"}
	c.Identity.HostKeyFingerprints = []string{"SHA256:" + id}
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1, AgentVersion: "1.6.0"}
	c.Resources.DiskFreeMB = fact(int64(100000))
	c.Tools.Toolchains = map[string]model.Fact[string]{"go": fact("1.25.3")}
	return Node{Cap: c, State: "online"}
}

func agentNode(t testing.TB, id string) Node {
	n := sshNode(t, id)
	n.Cap.Identity.Provider, n.Cap.Identity.Transport, n.Cap.Identity.SSH = "agent", "agent", nil
	return n
}

// healthy is a fleet where every check passes.
func healthy(t testing.TB) Facts {
	a, b := sshNode(t, "alpha"), agentNode(t, "bravo")
	return Facts{
		Now: t0, Nodes: []Node{a, b},
		SSHVersion:   remote.Version{Major: 9, Minor: 6},
		Reach:        map[string]error{"alpha": nil},
		Pins:         map[string][]string{"alpha": {"SHA256:alpha"}},
		CertExpiry:   map[string]time.Time{"bravo": t0.Add(5 * 24 * time.Hour)},
		Skew:         map[string]time.Duration{"alpha": time.Second},
		DiscoveryOff: true, MinAgentVersion: "1.5.0", MinProtocol: 1, PinnedRelease: "1.6.0",
	}
}

func byID(cs []doctor.Check, id string) []doctor.Check {
	var out []doctor.Check
	for _, c := range cs {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

func worst(cs []doctor.Check) doctor.Status {
	st := doctor.Pass
	for _, c := range cs {
		if c.Status == doctor.Fail {
			return doctor.Fail
		}
		if c.Status == doctor.Warn {
			st = doctor.Warn
		}
	}
	return st
}

func TestCatalogIsComplete(t *testing.T) {
	cat := Catalog()
	if len(cat) != 15 {
		t.Fatalf("catalog has %d rows, want 15", len(cat))
	}
	for i, e := range cat {
		want := "CI-DN" + string(rune('0'+(i+1)/100)) + string(rune('0'+(i+1)/10%10)) + string(rune('0'+(i+1)%10))
		if e.ID != want || e.Name == "" || e.Remediation == "" {
			t.Errorf("row %d = %+v, want id %s with a name and a remediation", i, e, want)
		}
	}
}

func TestHealthyFleetIsQuiet(t *testing.T) {
	out := Evaluate(healthy(t))
	for _, e := range Catalog() {
		rows := byID(out, e.ID)
		if len(rows) == 0 {
			t.Errorf("%s produced no result", e.ID)
		}
		if worst(rows) != doctor.Pass {
			t.Errorf("%s is not clean on a healthy fleet: %+v", e.ID, rows)
		}
	}
	for _, c := range out {
		if c.Remediation == "" || c.Name == "" {
			t.Errorf("%s has no name or remediation: %+v", c.ID, c)
		}
	}
	if doctor.ExitCode(out) != doctor.ExitClean {
		t.Errorf("exit code = %d", doctor.ExitCode(out))
	}
}

// TestEveryCheckFires breaks the healthy fleet one way per id and expects that id, and only a node subject, to go red.
func TestEveryCheckFires(t *testing.T) {
	cases := []struct {
		id   string
		want doctor.Status
		subj string
		mut  func(*Facts)
	}{
		{IDReach, doctor.Fail, "alpha", func(f *Facts) { f.Reach["alpha"] = errors.New("connection refused") }},
		{IDReach, doctor.Fail, "bravo", func(f *Facts) { f.Nodes[1].State = "offline" }},
		{IDReach, doctor.Warn, "", func(f *Facts) { f.StoreErr = errors.New("database is locked") }},
		{IDHostKey, doctor.Fail, "alpha", func(f *Facts) { f.Pins["alpha"] = nil }},
		{IDHostKey, doctor.Fail, "alpha", func(f *Facts) { f.Pins["alpha"] = []string{"SHA256:other"} }},
		{IDOpenSSH, doctor.Fail, "", func(f *Facts) { f.SSHVersion = remote.Version{Major: 7, Minor: 9} }},
		{IDOpenSSH, doctor.Fail, "", func(f *Facts) { f.SSHVersionErr = errors.New("no ssh") }},
		{IDHostKey, doctor.Fail, "alpha", func(f *Facts) { f.HostKeyRefused = map[string]bool{"alpha": true} }},
		{IDCert, doctor.Warn, "bravo", func(f *Facts) { f.CertExpiry["bravo"] = t0.Add(36 * time.Hour) }},
		{IDCert, doctor.Fail, "bravo", func(f *Facts) { f.CertExpiry["bravo"] = t0.Add(-time.Hour) }},
		{IDCert, doctor.Fail, "bravo", func(f *Facts) { delete(f.CertExpiry, "bravo") }},
		{IDSkew, doctor.Warn, "alpha", func(f *Facts) { f.Skew["alpha"] = -90 * time.Second }},
		{IDSkew, doctor.Fail, "alpha", func(f *Facts) { f.Skew["alpha"] = 121 * time.Second }},
		{IDVersion, doctor.Fail, "alpha", func(f *Facts) { f.Nodes[0].Cap.Protocol.AgentVersion = "1.4.9" }},
		{IDVersion, doctor.Fail, "alpha", func(f *Facts) { f.MinProtocol = 2 }},
		{IDVersion, doctor.Warn, "alpha", func(f *Facts) { f.Nodes[0].Cap.Protocol.AgentVersion = "" }},
		{IDDrift, doctor.Warn, "go", func(f *Facts) { f.Nodes[1].Cap.Tools.Toolchains["go"] = fact("1.24.1") }},
		{IDDisk, doctor.Warn, "alpha", func(f *Facts) { f.Nodes[0].Cap.Resources.DiskFreeMB = fact(int64(3000)) }},
		{IDDisk, doctor.Fail, "alpha", func(f *Facts) { f.Nodes[0].Cap.Resources.DiskFreeMB = fact(int64(500)) }},
		{IDDisk, doctor.Warn, "bravo", func(f *Facts) { f.ExpiredLeases = map[string]int{"bravo": 2} }},
		{IDDisk, doctor.Warn, "leases", func(f *Facts) { f.LeaseErr = errors.New("database is locked") }},
		{IDExposure, doctor.Fail, "alpha", func(f *Facts) {
			f.Nodes[0].Cap.Trust.Accepts = fact([]model.TrustClass{"untrusted"})
		}},
		{IDExposure, doctor.Fail, "alpha", func(f *Facts) {
			n := &f.Nodes[0].Cap
			n.Trust.Accepts = fact([]model.TrustClass{"owner", "untrusted"})
			n.Trust.Isolation = fact(model.Isolation("container"))
			n.Tools.Docker = fact(true)
			n.Separation.UserNS = fact(false)
		}},
		{IDDeploy, doctor.Fail, "alpha", func(f *Facts) {
			f.Nodes[0].Cap.DeployHost.Matched = true
			f.Nodes[0].Cap.Lifecycle.AuthorizedProjects = []string{"proj"}
		}},
		{IDDeploy, doctor.Warn, "exclusion", func(f *Facts) { f.ExclusionWarnings = []string{"deploy-host refresh failed for /x (boom)"} }},
		{IDBreaker, doctor.Warn, "alpha", func(f *Facts) { f.Nodes[0].State, f.Nodes[0].Reason = "maintenance", "3 infra failures" }},
		{IDDiscovery, doctor.Warn, "", func(f *Facts) { f.DiscoveryErr = errors.New("config unreadable") }},
		{IDUpgrade, doctor.Warn, "alpha", func(f *Facts) { f.PinnedRelease = "1.7.0" }},
		{IDGitHub, doctor.Fail, "alpha", func(f *Facts) {
			f.Nodes[0].Cap.Identity.Provider = "github-jit"
			f.Nodes[0].Cap.Availability.Persistence = fact(true)
			f.RepoVisibility = "public"
		}},
		{IDSeparate, doctor.Warn, "", func(f *Facts) { f.ModeErr = errors.New("database is locked") }},
		{IDGitHub, doctor.Warn, "alpha", func(f *Facts) {
			f.Nodes[0].Cap.Identity.Provider = "github-jit"
			f.Nodes[0].Cap.Availability.Persistence = fact(true)
		}},
		{IDSeparate, doctor.Fail, "alpha", func(f *Facts) {
			f.TeamMode = true
			f.Nodes[0].Cap.Separation.UIDSeparation = fact(false)
			f.Nodes[0].Cap.Lifecycle.AuthorizedProjects = []string{"proj"}
		}},
	}
	// An old agent is also a pending upgrade: one break, two honest findings.
	also := map[string]string{IDVersion: IDUpgrade}
	for _, tc := range cases {
		t.Run(tc.id+"/"+tc.subj+"/"+string(tc.want), func(t *testing.T) {
			f := healthy(t)
			tc.mut(&f)
			var hit *doctor.Check
			for _, c := range byID(Evaluate(f), tc.id) {
				if c.Status == tc.want && c.Subject == tc.subj {
					hit = &c
				}
			}
			if hit == nil {
				t.Fatalf("%s did not %s for subject %q: %+v", tc.id, tc.want, tc.subj, byID(Evaluate(f), tc.id))
			}
			if hit.Remediation == "" || hit.Message == "" {
				t.Errorf("no remediation on %+v", hit)
			}
			// No other id changed status: each fixture breaks exactly one thing.
			for _, e := range Catalog() {
				if e.ID != tc.id && e.ID != also[tc.id] && worst(byID(Evaluate(f), e.ID)) != doctor.Pass {
					t.Errorf("%s also went red: %+v", e.ID, byID(Evaluate(f), e.ID))
				}
			}
		})
	}
}

func TestNoNodesPassesEveryCheck(t *testing.T) {
	f := Facts{Now: t0, StoreNote: "no state database yet", SSHVersionErr: errors.New("no ssh"), DiscoveryOff: true}
	out := Evaluate(f)
	for _, e := range Catalog() {
		rows := byID(out, e.ID)
		if len(rows) == 0 || worst(rows) != doctor.Pass {
			t.Errorf("%s on an empty registry: %+v", e.ID, rows)
		}
	}
}

func TestPersistentGitHubRunnerOnPrivateRepoPasses(t *testing.T) {
	f := healthy(t)
	f.Nodes[0].Cap.Identity.Provider = "github-jit"
	f.Nodes[0].Cap.Availability.Persistence = fact(true)
	f.RepoVisibility = "private"
	if st := worst(byID(Evaluate(f), IDGitHub)); st != doctor.Pass {
		t.Fatalf("private repository = %s, want pass", st)
	}
}

func TestSmallCases(t *testing.T) {
	t.Run("old OpenSSH does not matter without ssh nodes", func(t *testing.T) {
		f := healthy(t)
		f.Nodes = f.Nodes[1:] // the agent node only
		f.SSHVersion = remote.Version{Major: 7, Minor: 4}
		if st := worst(byID(Evaluate(f), IDOpenSSH)); st != doctor.Pass {
			t.Fatalf("status = %s", st)
		}
	})
	t.Run("an unreachable ssh node says so instead of naming the transport", func(t *testing.T) {
		f := healthy(t)
		f.Reach["alpha"] = errors.New("refused")
		delete(f.Skew, "alpha")
		for _, c := range byID(Evaluate(f), IDSkew) {
			if c.Subject == "alpha" && !strings.Contains(c.Message, "unreachable") {
				t.Fatalf("message = %q", c.Message)
			}
		}
	})
	t.Run("an unreadable registry is named on every empty pass", func(t *testing.T) {
		f := Facts{Now: t0, StoreErr: errors.New("disk I/O error"), DiscoveryOff: true}
		for _, id := range []string{IDSkew, IDVersion, IDDisk, IDCert} {
			rows := byID(Evaluate(f), id)
			if len(rows) == 0 || !strings.Contains(rows[0].Message, "disk I/O error") {
				t.Errorf("%s = %+v, want the store error in the message", id, rows)
			}
		}
	})
	t.Run("discovery reports both states", func(t *testing.T) {
		f := healthy(t)
		if m := byID(Evaluate(f), IDDiscovery)[0].Message; !strings.Contains(m, "off") {
			t.Errorf("off message = %q", m)
		}
		f.DiscoveryOff = false
		if m := byID(Evaluate(f), IDDiscovery)[0].Message; !strings.Contains(m, "on") {
			t.Errorf("on message = %q", m)
		}
	})
}
