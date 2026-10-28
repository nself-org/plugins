//go:build security

package security

// Purpose: recorded payload to facts to class to placement, end to end (P7-HOST-05, TM-01, TM-32,
// TM-33). A recorded GitHub fork pull request delivery is authenticated with a per-repository
// secret, normalized, re-fetched from a fake GitHub API (recorded responses), classified, and the
// class steers a real dispatcher. Forged claims in the payload never raise the class; a bad or
// missing signature is refused with E706 before any event decision.
// Inputs:  the HOST-05 recorded deliveries and API responses (read from their testdata), synthetic secrets.
// Outputs: pass or fail.
// Constraints: no live network call; the fake API binds loopback TLS and the client trusts only its
//          certificate; every secret is an obviously synthetic string.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/approval"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

const (
	hookSecret = "synthetic-per-repo-secret"
	hookDir    = "../../internal/providers/github/webhook/testdata"
)

type route struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// githubRig is a webhook Source over a fake GitHub API serving the recorded responses.
type githubRig struct {
	src  *webhook.Source
	srv  *httptest.Server
	hits func() int
}

func newGithubRig(t *testing.T) *githubRig {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(hookDir, "api", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	routes := map[string]route{}
	if err = json.Unmarshal(raw, &routes); err != nil {
		t.Fatal(err)
	}
	var count atomic.Int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		rt, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.Status)
		_, _ = w.Write(rt.Body)
	}))
	t.Cleanup(srv.Close)
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	return &githubRig{srv: srv, hits: func() int { return int(count.Load()) }, src: &webhook.Source{
		Now:    func() time.Time { return now },
		Getenv: func(k string) string { return map[string]string{"SYNTH_REPO_SECRET": hookSecret}[k] },
		Secrets: func(context.Context) (map[string]webhook.RepoSecret, error) {
			return map[string]webhook.RepoSecret{"o/r": {Env: "SYNTH_REPO_SECRET"}}, nil
		},
		Insecure: func() bool { return false },
		NewAPI: func(context.Context) (*rest.Client, error) {
			return &rest.Client{BaseURL: srv.URL, AllowPrivateBase: true, Provider: webhook.Name, TLSConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig}, nil
		},
	}}
}

func hookFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(hookDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func hookRequest(event, signature string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	req.Header.Set("X-GitHub-Delivery", "d-1")
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	return req
}

// classify runs the whole pipeline of a delivery: Verify, Refetch, Classify.
func (g *githubRig) classify(t *testing.T, event string, body []byte) (model.TriggerFacts, source.Result) {
	t.Helper()
	ctx := context.Background()
	facts, err := g.src.Verify(ctx, hookRequest(event, sign(hookSecret, body), body), body)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	facts, err = g.src.Refetch(ctx, facts)
	if err != nil {
		t.Fatalf("refetch: %v", err)
	}
	return facts, source.Classify(facts)
}

// TestRecordedForkPRToClassToPlacement: the recorded fork PR is untrusted and held for approval,
// even when the payload lies about the author and the fork flag, and a dispatcher fed that class
// places it only where untrusted code may run.
func TestRecordedForkPRToClassToPlacement(t *testing.T) {
	g := newGithubRig(t)
	honest := hookFixture(t, "pr_fork_opened.json")
	forged := honest
	for _, edit := range [][2]string{{`"author_association":"FIRST_TIME_CONTRIBUTOR"`, `"author_association":"OWNER"`}, {`"fork":true`, `"fork":false`}, {`"login":"newbie"`, `"login":"maint"`}} {
		if !bytes.Contains(forged, []byte(edit[0])) {
			t.Fatalf("the recorded fixture no longer holds %s: the forgery would be weaker than it claims", edit[0])
		}
		forged = bytes.ReplaceAll(forged, []byte(edit[0]), []byte(edit[1]))
	}
	for name, body := range map[string][]byte{"recorded": honest, "payload claims owner, no fork, maintainer login": forged} {
		facts, res := g.classify(t, "pull_request", body)
		if res.Class != "untrusted" || !res.RequiresApproval {
			t.Fatalf("%s: class %q approval %v (reasons %v), want untrusted held for approval", name, res.Class, res.RequiresApproval, res.Reasons)
		}
		if f := facts.Fork; f.Value == nil || !*f.Value || f.Source != "api" {
			t.Fatalf("%s: the fork fact must come from the API as true: %+v", name, f)
		}
		// The class and the facts steer a real dispatcher: the run is held for its approval, and once the
		// approver has seen the head it lands on gVisor and not on the LAN node.
		r := newRig(t, repoPolicy(t, "public", res.Class), lanRunner("lan1", 2), gvisorRunner("gv1", 2))
		r.noFacts = true
		id, job := r.prepare(newJob("fromhook", res.Class, "restricted"), string(res.Class))
		head := ""
		if facts.HeadRepo.Value != nil {
			head = *facts.HeadRepo.Value
		}
		runID := "pipe-fromhook"
		if _, err := r.st.RecordApprovalFacts(context.Background(), store.ApprovalRunFacts{Run: runID, Author: "newbie", HeadRepo: head, RequiresApproval: res.RequiresApproval}, approval.VoidCheck); err != nil {
			t.Fatal(err)
		}
		a, err := r.enqueue(id, job, settle)
		heldBecause(t, r, id, a, err)
		if _, err = r.st.ApproveRun(context.Background(), store.ApproveInput{Run: runID, Approver: "token:t1:maintainer", ExpectSHA: baseRev, Check: approval.VoidCheck}); err != nil {
			t.Fatal(err)
		}
		a, err = r.enqueue(id, job, 2*time.Second)
		if got := runnerOf(t, a, err); got != "gv1" {
			t.Fatalf("%s: untrusted PR placed on %q, want gv1", name, got)
		}
	}
	// Control: a push by a repository admin is not blanket-untrusted.
	_, push := g.classify(t, "push", hookFixture(t, "push.json"))
	if trustRank(push.Class) <= trustRank("untrusted") {
		t.Fatalf("control: an admin push classified %q (%v)", push.Class, push.Reasons)
	}
}

func trustRank(c model.TrustClass) int {
	for i, v := range model.EnumValues("TrustClass") {
		if model.TrustClass(v) == c {
			return len(model.EnumValues("TrustClass")) - i
		}
	}
	return 0
}

// TestWebhookAuthenticity: an unsigned, wrongly signed or other-repository-signed delivery is
// refused with E706 before the API is asked anything (TM-33).
func TestWebhookAuthenticity(t *testing.T) {
	g := newGithubRig(t)
	body := hookFixture(t, "pr_fork_opened.json")
	for name, sig := range map[string]string{
		"no signature":          "",
		"wrong secret":          sign("synthetic-other-secret", body),
		"signature of old body": sign(hookSecret, append([]byte(nil), body[:len(body)-1]...)),
		"malformed header":      "sha256=zz",
	} {
		_, err := g.src.Verify(context.Background(), hookRequest("pull_request", sig, body), body)
		var pe *providers.Error
		if !errors.As(err, &pe) || pe.Code != "E706" || pe.Status != http.StatusForbidden {
			t.Fatalf("%s: got %v, want E706 403", name, err)
		}
	}
	if n := g.hits(); n != 0 {
		t.Fatalf("an unauthenticated delivery made %d API calls", n)
	}
	// An unknown repository with no shared secret and no --insecure is refused too.
	other := bytes.ReplaceAll(body, []byte(`"full_name":"o/r"`), []byte(`"full_name":"x/y"`))
	if _, err := g.src.Verify(context.Background(), hookRequest("pull_request", sign(hookSecret, other), other), other); err == nil || !strings.Contains(err.Error(), "E706") {
		t.Fatalf("a delivery for a repository with no secret: %v", err)
	}
}
