package attest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// leaky is evidence full of things a public document must not carry.
func leaky() model.Evidence {
	ev := goodEvidence()
	ev.Runner.ID = "mac-mini.local"
	ev.Checks[0].Excerpt = "connect to 192.168.1.20:5432 failed\ndial 10.0.0.7 timeout\nrunner mac-mini.local at /Users/ali/Sites/nself/x.go\n" +
		"ssh ali@build-box.lan\nsee https://ci.corp.local/runs/9?token=abc and https://github.com/nself-org/plugins/actions/runs/9\n" +
		"proxy 172.16.4.4 and 172.31.255.1 and fe80::1 and fd12:3456::9 and 127.0.0.1 and 8.8.8.8 at C:\\Users\\bob\\work"
	ev.Checks[1].LogRef = strp("/home/runner/work/log.txt")
	ev.Checks[1].ToolVersion = strp("1.2.3 on host dev-box.lan")
	ev.Artifacts = []model.Artifact{{Path: "/Users/ali/out/app.tar", SHA256: strings.Repeat("a", 64), Size: 1, Committed: true}}
	ev.ToolVersions = map[string]string{"go": "1.27.2", "node": "v22 (/Users/ali/.nvm)"}
	ev.PlacementRef = strp("node mac-mini.local slot 2")
	return ev
}

func TestPublish_PublicRemovesPrivateData(t *testing.T) {
	ev := leaky()
	out := attest.Publish(ev, attest.Public)
	doc, _ := json.Marshal(out)
	text := string(doc)
	for _, must := range []string{"192.168.1.20", "10.0.0.7", "mac-mini", "/Users/ali", "ali@", "ci.corp.local", "token=abc", "172.16.4.4",
		"172.31.255.1", "fe80::1", "fd12:3456", "127.0.0.1", "C:\\\\Users\\\\bob", "build-box", "/home/runner", "dev-box.lan"} {
		if strings.Contains(text, must) {
			t.Errorf("published document still holds %q:\n%s", must, text)
		}
	}
	for _, keep := range []string{"8.8.8.8", "https://github.com/nself-org/plugins/actions/runs/9", "go.sum"} {
		if !strings.Contains(text, keep) {
			t.Errorf("published document lost %q", keep)
		}
	}
}

func TestPublish_PrivateRepoIsUnchanged(t *testing.T) {
	ev := leaky()
	before := string(marshal(t, ev))
	if after := string(marshal(t, attest.Publish(ev, attest.Private))); after != before {
		t.Fatalf("private evidence changed:\n%s\n%s", before, after)
	}
}

func TestPublish_UnknownVisibilityScrubs(t *testing.T) {
	for _, v := range []attest.Visibility{"", "internal", "PRIVATE", "Private "} {
		if out := attest.Publish(leaky(), v); out.Runner.ID == "mac-mini.local" {
			t.Errorf("visibility %q published raw", v)
		}
	}
}

func TestPublish_DoesNotModifyItsInput(t *testing.T) {
	ev := leaky()
	before := string(marshal(t, ev))
	attest.Publish(ev, attest.Public)
	if string(marshal(t, ev)) != before {
		t.Fatal("Publish modified its input")
	}
}

func TestPublish_RunnerPseudonymIsStableAndSeparates(t *testing.T) {
	a := attest.Publish(leaky(), attest.Public)
	b := attest.Publish(leaky(), attest.Public)
	if a.Runner.ID != b.Runner.ID || !strings.HasPrefix(a.Runner.ID, "runner-") || len(a.Runner.ID) != len("runner-")+12 {
		t.Fatalf("%q %q", a.Runner.ID, b.Runner.ID)
	}
	other := leaky()
	other.Runner.ID = "mac-studio.local"
	if attest.Publish(other, attest.Public).Runner.ID == a.Runner.ID {
		t.Fatal("two runners share a pseudonym")
	}
	otherRepo := leaky()
	otherRepo.RepoID = "repo-2"
	if attest.Publish(otherRepo, attest.Public).Runner.ID == a.Runner.ID {
		t.Fatal("pseudonyms correlate across repositories")
	}
}

// ProfileFor scrubs exactly as Publish does (CACHE delta 8): one table.
func TestProfileFor_MatchesPublish(t *testing.T) {
	ev := leaky()
	pub := attest.Publish(ev, attest.Public)
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public, RepoID: ev.RepoID})
	if !p.Public() {
		t.Fatal("public profile reports private")
	}
	if p.RunnerID(ev.Runner.ID) != pub.Runner.ID {
		t.Fatalf("runner %q vs %q", p.RunnerID(ev.Runner.ID), pub.Runner.ID)
	}
	if p.CoordinatorID("coord-1") == "coord-1" || !strings.HasPrefix(p.CoordinatorID("coord-1"), "coordinator-") || p.CoordinatorID("coord-1") != p.CoordinatorID("coord-1") {
		t.Fatalf("coordinator %q", p.CoordinatorID("coord-1"))
	}
	if p.CoordinatorID("x") == p.RunnerID("x") {
		t.Fatal("runner and coordinator pseudonyms collide")
	}
	// the shared table: whatever Publish removes from free text, Text removes too
	for _, banned := range []string{"192.168.1.20", "10.0.0.7", "/Users/ali", "ali@", "ci.corp.local", "token=abc", "172.16.4.4", "fe80::1", "127.0.0.1", "build-box"} {
		if got := p.Text(ev.Checks[0].Excerpt); strings.Contains(got, banned) {
			t.Fatalf("profile text leaks %q: %s", banned, got)
		}
		if strings.Contains(pub.Checks[0].Excerpt, banned) {
			t.Fatalf("publish leaks %q", banned)
		}
	}
	private := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Private, RepoID: ev.RepoID})
	if private.Public() || private.RunnerID("r") != "r" || private.CoordinatorID("c") != "c" || private.Text("10.0.0.1 /Users/a") != "10.0.0.1 /Users/a" {
		t.Fatal("private profile scrubs")
	}
	var zero attest.Profile
	if !zero.Public() || zero.RunnerID("r") == "r" {
		t.Fatal("zero profile must scrub")
	}
}

func TestProfile_URL(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public, RepoID: "r", Hostnames: []string{"ci.example.org"}})
	rows := map[string]string{
		"https://github.com/o/r/actions/runs/1?x=1#f": "https://github.com/o/r/actions/runs/1",
		"http://192.168.0.5/run/1":                    "",
		"https://ci.corp.local/x":                     "",
		"https://user:pw@github.com/x":                "",
		"https://ci.example.org/x":                    "",
		"https://localhost:8080/x":                    "",
		"https://[fd00::1]/x":                         "",
		"https://buildbox/x":                          "",
		"ftp://github.com/x":                          "",
		"file:///Users/a/x":                           "",
		"not a url":                                   "",
		"":                                            "",
	}
	for in, want := range rows {
		if got := p.URL(in); got != want {
			t.Errorf("URL(%q) = %q, want %q", in, got, want)
		}
	}
	private := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Private})
	if private.URL("http://10.0.0.1/x?y=1") != "http://10.0.0.1/x?y=1" {
		t.Fatal("private URL changed")
	}
}

func TestProfile_TextDetails(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public, RepoID: "r", Hostnames: []string{"gandalf"}, Usernames: []string{"frodo"}})
	rows := []struct{ in, want string }{
		{"pkg left-pad@1.2.3 and golang.org/x/net@v0.1.0", "pkg left-pad@1.2.3 and golang.org/x/net@v0.1.0"},
		{"git@github.com:org/repo.git", "git@github.com:org/repo.git"},
		{"mail bob@example.com", "mail [redacted-user]@example.com"},
		{"std::vector and a::b", "std::vector and a::b"},
		{"time 12:34:56", "time 12:34:56"},
		{"version 1.2.3.4", "version 1.2.3.4"},
		{"ip 100.64.0.1 and 169.254.1.1", "ip [redacted-ip] and [redacted-ip]"},
		{"ip 172.15.0.1 and 172.32.0.1", "ip 172.15.0.1 and 172.32.0.1"},
		{"gandalf and frodo, frodoish", "[redacted-host] and [redacted-user], frodoish"},
		{"/root/.cache and /home/sam/x", "[redacted-home]/.cache and [redacted-home]/x"},
		{"user sam wrote", "user [redacted-user] wrote"},
	}
	for _, r := range rows {
		got := p.Text(r.in)
		if r.in == "user sam wrote" {
			got = p.Text("/home/sam/x " + r.in)
			r.want = "[redacted-home]/x user [redacted-user] wrote"
		}
		if got != r.want {
			t.Errorf("Text(%q) = %q, want %q", r.in, got, r.want)
		}
	}
	if p.Text("") != "" {
		t.Fatal("empty text changed")
	}
	if a, b := p.Text("10.1.2.3 /Users/x/y"), p.Text("10.1.2.3 /Users/x/y"); a != b {
		t.Fatal("Text is not deterministic")
	}
}

func TestPublish_FreshnessIsUnknownAndGateNeedsRecompute(t *testing.T) {
	out := attest.Publish(leaky(), attest.Public)
	if out.Freshness.State != "unknown" {
		t.Fatalf("%+v", out.Freshness)
	}
	// the scrubbed copy still gates only against recomputed digests
	g := goodGate(out)
	g.Current = nil
	if got := attest.GateAccept(out, g); !hasReason(got, attest.ReasonStale) {
		t.Fatalf("%v", got)
	}
}

func TestPublish_PublishedDocumentStillValidatesAgainstTheSchema(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	if _, err := attest.Verify(marshal(t, attest.Publish(leaky(), attest.Public))); err != nil {
		t.Fatalf("published copy is not a valid document: %v", err)
	}
}
