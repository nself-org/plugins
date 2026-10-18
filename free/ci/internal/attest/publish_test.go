package attest_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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

// A runner id such as "linux" must not rewrite structured fields.
func TestPublish_RunnerAliasOnlyInFreeText(t *testing.T) {
	ev := goodEvidence()
	ev.Runner = model.RunnerRef{ID: "linux", Provider: "linux-pool", Platform: "linux/amd64", Isolation: "process"}
	ev.Checks[0].Excerpt = "linux box failed"
	out := attest.Publish(ev, attest.Public)
	if out.Runner.Platform != "linux/amd64" || out.Runner.Provider != "linux-pool" || out.Runner.ID == "linux" {
		t.Fatalf("%+v", out.Runner)
	}
	if strings.Contains(out.Checks[0].Excerpt, "linux box") {
		t.Fatalf("runner id left in free text: %q", out.Checks[0].Excerpt)
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
		"https://github.com/o/r/actions/runs/1?x=1#f":      "https://github.com/o/r/actions/runs/1",
		"http://192.168.0.5/run/1":                         "",
		"https://ci.corp.local/x":                          "",
		"https://" + "user" + ":" + "pw" + "@github.com/x": "",
		"https://ci.example.org/x":                         "",
		"https://localhost:8080/x":                         "",
		"https://[fd00::1]/x":                              "",
		"https://buildbox/x":                               "",
		"ftp://github.com/x":                               "",
		"file:///Users/a/x":                                "",
		"not a url":                                        "",
		"":                                                 "",
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
		{"GANDALF and Frodo", "[redacted-host] and [redacted-user]"},
		{"ip 010.0.0.1 and 192.168.01.5 and 8.8.8.8", "ip [redacted-ip] and [redacted-ip] and 8.8.8.8"},
		{"v6 64:ff9b::a00:1 and 2002:c0a8:101::1", "v6 [redacted-ip] and [redacted-ip]"},
		{"see https://github.com/o/r/runs/1?token=abc#x.", "see https://github.com/o/r/runs/1."},
		{"cd /home/runner/work and run runner tests in /Users/build/x", "cd [redacted-home]/work and run runner tests in [redacted-home]/x"},
		{"self.private app.internal cfg.home", "[redacted-host] [redacted-host] [redacted-host]"},
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

// Every free string of the document is scrubbed, not just the ones with an obvious host: check ids, job ids,
// producer version, run ids and the provider attestation (review round 2).
func TestPublish_ScrubsEveryFreeString(t *testing.T) {
	ev := goodEvidence()
	const leak = "/home/alice/10.0.0.7"
	ev.Checks[0].ID, ev.Checks[0].JobID, ev.Checks[0].Kind = leak, leak, leak
	ev.Producer.Version, ev.RunID, ev.Pipeline.ID = leak, leak, leak
	ev.Job, ev.Attempt = strp(leak), &model.AttemptRef{ID: leak, N: 1}
	ev.Retries = []model.Retry{{JobID: leak, Attempt: 1, Class: "infra"}}
	ev.ToolVersions = map[string]string{leak: leak}
	ev.ProviderAttestation = &model.ProviderAttestation{Provider: "github", RunID: leak, Ref: leak, WorkflowPath: leak, TemplateVersion: leak, ArtifactID: leak,
		Nonce: leak, VerifiedAt: leak, WorkflowSHA: leak, WorkflowDigest: leak, ArtifactSHA256: leak}
	before := string(marshal(t, ev))
	out := attest.Publish(ev, attest.Public)
	if string(marshal(t, ev)) != before {
		t.Fatal("Publish modified its input")
	}
	for _, bad := range []string{"alice", "10.0.0.7", "/home/"} {
		if text := string(marshal(t, out)); strings.Contains(text, bad) {
			t.Errorf("published document still holds %q:\n%s", bad, text)
		}
	}
}

// A check id equal to the runner id is pseudonymised like the runner id itself.
func TestPublish_CheckIDEqualToRunnerID(t *testing.T) {
	ev := goodEvidence()
	ev.Runner.ID = "remote-ci-265"
	ev.Checks[0].ID = "remote-ci-265"
	out := attest.Publish(ev, attest.Public)
	if out.Checks[0].ID == "remote-ci-265" || out.Checks[0].ID != out.Runner.ID {
		t.Fatalf("check id %q runner %q", out.Checks[0].ID, out.Runner.ID)
	}
}

// Profile.URL drops a URL whose path names a user, host or private address,
// in any percent-encoding depth.
func TestProfileURL_PrivatePathDropped(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public, Usernames: []string{"alice"}, Hostnames: []string{"runner.example.org"}})
	for _, u := range []string{
		"https://logs.example.com/home/alice/runner.example.org/10.0.0.7",
		"https://logs.example.com/Users/bob/x",
		"https://logs.example.com/runs/192.168.1.5/log",
		"https://logs.example.com/%2Fhome%2Fbob%2Fx",
		"https://logs.example.com/%252Fhome%252Fbob",
		"https://logs.example.com/a/alice/b",
		"https://logs.example.com/a/runner.example.org",
		"https://logs.example.com/a/%2525252Fx",
		"https://logs.example.com/a/%zz",
	} {
		if got := p.URL(u); got != "" {
			t.Errorf("URL(%q) = %q, want dropped", u, got)
		}
	}
	for _, u := range []string{"https://github.com/nself-org/plugins/actions/runs/9", "https://logs.example.com/runs/42/log"} {
		if p.URL(u) != u {
			t.Errorf("URL(%q) = %q, want kept", u, p.URL(u))
		}
	}
}

// Profile.Text redacts every private suffix URL() treats as private (one list).
func TestProfileText_PrivateSuffixes(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	for _, h := range []string{"db01.corp", "mm4.home", "api.localhost", "vault.private", "nas.home.arpa", "x.intranet", "a.b.localdomain", "c.lan"} {
		if got := p.Text("ssh to " + h + ":22 failed"); strings.Contains(got, h) {
			t.Errorf("Text left %q: %q", h, got)
		}
	}
	if got := p.Text("see github.com and example.org"); got != "see github.com and example.org" {
		t.Errorf("public hosts changed: %q", got)
	}
}

// Nested URLs in a URL path must not make Text super-linear (review: k=48 took 22s).
func TestProfileText_NestedURLsAreBounded(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	hostile := "https://e.com" + strings.Repeat("/%2541/https://e.com", 200) + "/x"
	start := time.Now()
	_ = p.Text(hostile)
	_ = p.URL(hostile)
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("hostile nested URL took %v", d)
	}
	if p.URL(hostile) != "" {
		t.Fatal("a URL whose path holds another URL must be dropped")
	}
}

// A user name learned from a home path inside a dropped URL is still redacted in the rest of the text.
func TestProfileText_UserFromDroppedURLPath(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	got := p.Text("https://logs.example.org/home/alice/log user alice")
	if strings.Contains(got, "alice") {
		t.Fatalf("user survived: %q", got)
	}
}

func TestProfileText_UserFromDroppedURLVariants(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	for _, in := range []string{
		"https://logs.example.org/home/alice?download=1 user alice",
		"https://logs.example.org/%68ome/alice/log user alice",
		"https://logs.example.org/%2Fhome%2Falice#frag user alice",
	} {
		if got := p.Text(in); strings.Contains(got, "alice") {
			t.Errorf("Text(%q) = %q", in, got)
		}
	}
}

// Many dropped URLs naming one user stay linear (review round 4: 100000 copies).
func TestProfileText_RepeatedDroppedURLsAreBounded(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	start := time.Now()
	out := p.Text(strings.Repeat("https://logs.example.org/home/alice/log ", 20000) + "user alice")
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("took %v", d)
	}
	if strings.Contains(out, "alice") {
		t.Fatal("user survived")
	}
}

func TestProfileText_UserFromQueryAndCap(t *testing.T) {
	p := attest.ProfileFor(attest.RepoFacts{Visibility: attest.Public})
	if got := p.Text("https://logs.example.org/?path=/home/alice/log user alice"); strings.Contains(got, "alice") {
		t.Errorf("query user survived: %q", got)
	}
	var b strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, "/home/u%03d/x ", i)
	}
	b.WriteString("/home/alice/x user alice")
	if got := p.Text(b.String()); strings.Contains(got, "alice") {
		t.Errorf("cap overflow leaked: %q", got)
	}
}
