package webhook

// Purpose: authentication tests of the GitLab source (P7-HOST-21 acceptance line 1): Standard Webhooks
// signatures, the bare X-Gitlab-Token, the no-downgrade rule and the project identity binding.
// Inputs:  the recorded deliveries and a fake GitLab API.
// Outputs: pass or fail.
// Constraints: every secret is synthetic; time is the rig's injected clock.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// wantReject fails unless err is an E706 403 with the given reason.
func wantReject(t *testing.T, what string, err error, reason string) {
	t.Helper()
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("%s: err = %v, want E706 %s", what, err, reason)
	}
	if pe.Code != "E706" || pe.Reason != reason || pe.Status != http.StatusForbidden || pe.Class != providers.Auth {
		t.Fatalf("%s: got %+v, want E706 %s 403", what, pe, reason)
	}
}

// TestVerifyAuthFacts proves acceptance line 1: a valid Standard Webhooks signature yields auth
// per-repo; a valid X-Gitlab-Token with no signature yields auth bearer for a project without a
// signing secret and E706 signature for a project with one; a signature over a tampered body, a
// timestamp 6 minutes old and a wrong token each yield E706 with their reason and a 403 status.
func TestVerifyAuthFacts(t *testing.T) {
	push := fixture(t, "push")
	tokened := replace(push, `"path_with_namespace":"acme/app"`, `"path_with_namespace":"acme/tokened"`)
	t.Run("valid signature is per-repo", func(t *testing.T) {
		rg := newRig(t, both())
		f, err := rg.signed("Push Hook", push)
		if err != nil || f.Auth != "per-repo" || f.Repo != "acme/app" || f.RepoID != "1001" || f.DeliveryID != "msg-1" || f.Source != "gitlab" {
			t.Fatalf("facts = %+v err = %v", f, err)
		}
		if f.Validate() != nil {
			t.Fatalf("facts must validate: %v", f.Validate())
		}
	})
	t.Run("valid token without a signing secret is bearer", func(t *testing.T) {
		rg := newRig(t, both())
		f, err := rg.bearer("Push Hook", bareToken, tokened)
		if err != nil || f.Auth != "bearer" || f.Repo != "acme/tokened" || f.DeliveryID != "uuid-1" {
			t.Fatalf("facts = %+v err = %v", f, err)
		}
	})
	t.Run("token for a project with a signing secret", func(t *testing.T) {
		rg := newRig(t, both())
		_, err := rg.bearer("Push Hook", bareToken, push)
		wantReject(t, "bare token on a signing project", err, reasonSignature)
	})
	t.Run("tampered body", func(t *testing.T) {
		rg := newRig(t, both())
		d := delivery{event: "Push Hook", id: "msg-1", ts: rg.now, signSecret: signingSecret}
		req := d.request(push)
		tampered := replace(push, `"user_username":"dev1"`, `"user_username":"evil"`)
		_, err := rg.src.Verify(context.Background(), req, tampered)
		wantReject(t, "tampered body", err, reasonSignature)
	})
	t.Run("wrong key", func(t *testing.T) {
		rg := newRig(t, both())
		d := delivery{event: "Push Hook", id: "msg-1", ts: rg.now, signSecret: otherSecret}
		_, err := rg.src.Verify(context.Background(), d.request(push), push)
		wantReject(t, "signature under another key", err, reasonSignature)
	})
	t.Run("timestamp window", func(t *testing.T) {
		rg := newRig(t, both())
		for _, c := range []struct {
			age  time.Duration
			want string
		}{{5 * time.Minute, ""}, {-5 * time.Minute, ""}, {4*time.Minute + 59*time.Second, ""}, {-4*time.Minute - 59*time.Second, ""},
			{5*time.Minute + time.Second, reasonTimestamp}, {-5*time.Minute - time.Second, reasonTimestamp}, {6 * time.Minute, reasonTimestamp}, {-6 * time.Minute, reasonTimestamp}, {30 * 24 * time.Hour, reasonTimestamp}} {
			d := delivery{event: "Push Hook", id: "msg-1", ts: rg.now.Add(-c.age), signSecret: signingSecret}
			_, err := rg.src.Verify(context.Background(), d.request(push), push)
			if c.want == "" {
				if err != nil {
					t.Fatalf("age %v must pass: %v", c.age, err)
				}
				continue
			}
			wantReject(t, "age "+c.age.String(), err, c.want)
		}
	})
	t.Run("wrong and missing token", func(t *testing.T) {
		rg := newRig(t, both())
		_, err := rg.bearer("Push Hook", "synthetic-wrong-token", tokened)
		wantReject(t, "wrong token", err, reasonToken)
		_, err = rg.bearer("Push Hook", "", tokened)
		wantReject(t, "missing token", err, reasonToken)
		rg.env[tokenEnv] = "" // an unset variable never matches an empty header either
		_, err = rg.bearer("Push Hook", "", tokened)
		wantReject(t, "unset token variable", err, reasonToken)
	})
	t.Run("unconfigured project and unreadable policy fail closed", func(t *testing.T) {
		rg := newRig(t, Secrets{})
		_, err := rg.signed("Push Hook", push)
		wantReject(t, "no secrets at all", err, reasonSignature)
		rg = newRig(t, both())
		rg.src.Policy = func(context.Context) (Secrets, error) { return Secrets{}, errors.New("policy unreadable") }
		_, err = rg.signed("Push Hook", push)
		wantReject(t, "unreadable policy", err, reasonSignature)
		rg = newRig(t, both())
		rg.env[signingEnv] = "" // the variable named by policy is unset
		_, err = rg.signed("Push Hook", push)
		wantReject(t, "unset signing variable", err, reasonSignature)
		rg.env[signingEnv] = "not-a-whsec-secret"
		_, err = rg.signed("Push Hook", push)
		wantReject(t, "malformed signing secret", err, reasonSignature)
	})
	t.Run("a list of signatures", func(t *testing.T) {
		rg := newRig(t, both())
		ts := strconv.FormatInt(rg.now.Unix(), 10)
		good := stdSignature(signingSecret, "msg-1", ts, push)
		for _, c := range []struct {
			header string
			ok     bool
		}{
			{"v1,AAAA " + good, true}, {good + " v1,AAAA", true}, {"v2,AAAA " + good, true}, {"v2," + good[3:], false},
			{good[3:], false}, {"v1,", false}, {"v1,!!!", false}, {"", false},
		} {
			d := delivery{event: "Push Hook", id: "msg-1", ts: rg.now, signature: c.header}
			_, err := rg.src.Verify(context.Background(), d.request(push), push)
			if (err == nil) != c.ok {
				t.Fatalf("header %q: err = %v, want ok=%v", c.header, err, c.ok)
			}
		}
	})
	t.Run("a MAC with any byte changed", func(t *testing.T) {
		rg := newRig(t, both())
		ts := strconv.FormatInt(rg.now.Unix(), 10)
		key, _ := decodeKey(signingSecret)
		m := hmac.New(sha256.New, key)
		m.Write(signedContent("msg-1", ts, push))
		mac := m.Sum(nil)
		for i := range mac { // the first byte, the last byte and every one between
			bad := append([]byte{}, mac...)
			bad[i] ^= 0x01
			d := delivery{event: "Push Hook", id: "msg-1", ts: rg.now, signature: "v1," + base64.StdEncoding.EncodeToString(bad)}
			_, err := rg.src.Verify(context.Background(), d.request(push), push)
			wantReject(t, "MAC with byte "+strconv.Itoa(i)+" changed", err, reasonSignature)
		}
		short := delivery{event: "Push Hook", id: "msg-1", ts: rg.now, signature: "v1," + base64.StdEncoding.EncodeToString(mac[:31])}
		_, err := rg.src.Verify(context.Background(), short.request(push), push)
		wantReject(t, "truncated MAC", err, reasonSignature)
	})
	t.Run("a signature for another delivery id or timestamp", func(t *testing.T) {
		rg := newRig(t, both())
		ts := strconv.FormatInt(rg.now.Unix(), 10)
		d := delivery{event: "Push Hook", id: "msg-2", ts: rg.now, signature: stdSignature(signingSecret, "msg-1", ts, push)}
		_, err := rg.src.Verify(context.Background(), d.request(push), push)
		wantReject(t, "signature of another id", err, reasonSignature)
		d = delivery{event: "Push Hook", id: "msg-1", ts: rg.now, signature: stdSignature(signingSecret, "msg-1", strconv.FormatInt(rg.now.Unix()-1, 10), push)}
		_, err = rg.src.Verify(context.Background(), d.request(push), push)
		wantReject(t, "signature of another timestamp", err, reasonSignature)
	})
}

// TestNoBearerDowngrade proves acceptance line 1 and review F2: a project that has a signing secret
// never accepts the bare token, however the signature is dropped or damaged, and a project listed
// under both keys behaves as a signing project.
func TestNoBearerDowngrade(t *testing.T) {
	push := fixture(t, "push")
	rg := newRig(t, Secrets{
		Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv}},
		Tokens:  map[string]RepoSecret{"acme/app": {Env: tokenEnv}}, // the same project under both keys
	})
	for name, d := range map[string]delivery{
		"token only":             {event: "Push Hook", token: bareToken},
		"token with ids":         {event: "Push Hook", id: "msg-1", ts: rg.now, token: bareToken},
		"stripped signature":     {event: "Push Hook", id: "msg-1", ts: rg.now, token: bareToken, signSecret: ""},
		"damaged signature":      {event: "Push Hook", id: "msg-1", ts: rg.now, token: bareToken, signature: "v1,AAAA"},
		"token and wrong sig":    {event: "Push Hook", id: "msg-1", ts: rg.now, token: bareToken, signSecret: otherSecret},
		"stale signed + token":   {event: "Push Hook", id: "msg-1", ts: rg.now.Add(-time.Hour), token: bareToken, signSecret: signingSecret},
		"token with an event id": {event: "Push Hook", uuid: "u", token: bareToken},
	} {
		_, err := rg.src.Verify(context.Background(), d.request(push), push)
		if err == nil {
			t.Fatalf("%s: a signing project accepted a delivery it must refuse", name)
		}
		var pe *providers.Error
		if !errors.As(err, &pe) || pe.Code != "E706" || pe.Status != http.StatusForbidden {
			t.Fatalf("%s: err = %v", name, err)
		}
		if pe.Reason == reasonToken {
			t.Fatalf("%s: the token path ran for a signing project", name)
		}
	}
	f, err := rg.signed("Push Hook", push) // the signed delivery still works, and is per-repo
	if err != nil || f.Auth != "per-repo" {
		t.Fatalf("signed delivery = %+v %v", f, err)
	}
}

// TestIdentityBinding proves the review F2 binding (acceptance line 1, per-repo): the project name is
// bound to its numeric id, by configuration or by the first verified id, and a name outside
// group/project is refused.
func TestIdentityBinding(t *testing.T) {
	push := fixture(t, "push")
	t.Run("configured id", func(t *testing.T) {
		rg := newRig(t, Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv, ID: 4242}}})
		_, err := rg.signed("Push Hook", push)
		wantReject(t, "project id differs from the configured id", err, reasonSignature)
		rg = newRig(t, Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv, ID: 1001}}})
		if _, err = rg.signed("Push Hook", push); err != nil {
			t.Fatalf("configured id matches: %v", err)
		}
	})
	t.Run("first verified id is pinned", func(t *testing.T) {
		rg := newRig(t, both())
		if _, err := rg.signed("Push Hook", push); err != nil {
			t.Fatal(err)
		}
		recreated := replace(replace(push, `"id":1001,"name"`, `"id":7007,"name"`), `"project_id":1001`, `"project_id":7007`)
		_, err := rg.signed("Push Hook", recreated)
		wantReject(t, "project recreated under the same name", err, reasonSignature)
	})
	t.Run("a bearer delivery never pins an id nor addresses the API by one", func(t *testing.T) {
		rg := newRig(t, both())
		tokened := replace(fixture(t, "push"), `"path_with_namespace":"acme/app"`, `"path_with_namespace":"acme/tokened"`)
		claim, err := rg.bearer("Push Hook", bareToken, replace(tokened, `"id":1001,"name"`, `"id":666,"name"`))
		if err != nil || claim.RepoID != "" {
			t.Fatalf("an id nothing bound to the name must not be recorded: %+v %v", claim, err)
		}
		for _, p := range rg.gl.paths {
			if strings.Contains(p, "666") {
				t.Fatalf("the sender's claimed id addressed the API: %s", p)
			}
		}
		if _, err = rg.src.Refetch(context.Background(), claim); err != nil && rg.gl.saw("/projects/666") {
			t.Fatal("refetch used the claimed id")
		}
		for _, p := range rg.gl.paths {
			if strings.Contains(p, "666") {
				t.Fatalf("the sender's claimed id addressed the API on refetch: %s", p)
			}
		}
		if f, err := rg.bearer("Push Hook", bareToken, tokened); err != nil {
			t.Fatalf("the project's real delivery was locked out by an earlier claim: %+v %v", f, err)
		}
	})
	t.Run("a configured id binds a bearer delivery", func(t *testing.T) {
		rg := newRig(t, Secrets{Tokens: map[string]RepoSecret{"acme/tokened": {Env: tokenEnv, ID: 1001}}})
		tokened := replace(fixture(t, "push"), `"path_with_namespace":"acme/app"`, `"path_with_namespace":"acme/tokened"`)
		f, err := rg.bearer("Push Hook", bareToken, tokened)
		if err != nil || f.RepoID != "1001" || f.Auth != "bearer" {
			t.Fatalf("configured id: %+v %v", f, err)
		}
		_, err = rg.bearer("Push Hook", bareToken, replace(tokened, `"id":1001,"name"`, `"id":666,"name"`))
		wantReject(t, "claimed id differs from the configured id", err, reasonSignature)
	})
	t.Run("name case does not matter", func(t *testing.T) {
		rg := newRig(t, both())
		f, err := rg.signed("Push Hook", replace(push, `"acme/app"`, `"Acme/App"`))
		if err != nil || f.Repo != "acme/app" {
			t.Fatalf("facts = %+v err = %v", f, err)
		}
	})
	t.Run("nested groups are not enqueueable", func(t *testing.T) {
		rg := newRig(t, Secrets{Signing: map[string]RepoSecret{"acme/app": {Env: signingEnv}}})
		_, err := rg.signed("Push Hook", replace(push, `"acme/app"`, `"acme/sub/app"`))
		wantReject(t, "three-level path", err, reasonSignature)
	})
	t.Run("no project id", func(t *testing.T) {
		rg := newRig(t, both())
		_, err := rg.signed("Push Hook", replace(push, `"id":1001,"name"`, `"id":0,"name"`))
		wantReject(t, "id 0", err, reasonSignature)
	})
}

// TestKeysDecode proves the two operator keys accept a variable name or {env, id} and refuse anything
// that could be a secret value, a bad project name or a duplicate.
func TestKeysDecode(t *testing.T) {
	for _, k := range Keys() {
		good := []string{`{"acme/app":"MY_SECRET"}`, `{"acme/app":{"env":"MY_SECRET","id":9}}`, `{}`}
		for _, g := range good {
			if _, err := k.Decode([]byte(g)); err != nil {
				t.Fatalf("%s: %s refused: %v", k.Key, g, err)
			}
		}
		for _, bad := range []string{`null`, `[]`, `"x"`, `{"acme":"S"}`, `{"a/b/c":"S"}`, `{"acme/app":"has space"}`, `{"acme/app":"whsec_abc="}`,
			`{"acme/app":{"env":"S","id":-1}}`, `{"acme/app":{"env":"S","extra":1}}`, `{"acme/app":"S","Acme/App":"T"}`, `{"acme/app":""}`, `{"acme/app":1}`, `{} {}`} {
			if _, err := k.Decode([]byte(bad)); err == nil {
				t.Fatalf("%s: %s must be refused", k.Key, bad)
			}
		}
	}
	if k := Keys(); len(k) != 2 || k[0].Key != "providers.gitlab.webhook_signing_secrets" || k[1].Key != "providers.gitlab.webhook_tokens" {
		t.Fatalf("keys = %+v", k)
	}
}

// TestNoPreAuthAmplification proves the unauthenticated path allocates little however the body is
// shaped (Opus review M1): a body that is mostly a long list of empty strings is refused for about
// the cost of the identity probe, not of decoding the list.
func TestNoPreAuthAmplification(t *testing.T) {
	body := []byte(`{"tag_list":[` + strings.Repeat(`"",`, 1<<20) + `""],"runner":{"tags":[` + strings.Repeat(`"",`, 1<<18) + `""]}}`)
	rg := newRig(t, both())
	d := delivery{event: "Job Hook", id: "msg-1", ts: rg.now, signSecret: signingSecret}
	req := d.request(body)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := rg.src.Verify(context.Background(), req, body)
	runtime.ReadMemStats(&after)
	wantReject(t, "slice-heavy body without a project", err, reasonSignature)
	if grew := after.TotalAlloc - before.TotalAlloc; grew > uint64(len(body))/4 {
		t.Fatalf("an unauthenticated %d byte body allocated %d bytes: the pre-auth decode must not materialize its lists", len(body), grew)
	}
}
