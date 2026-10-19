package signing_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

// answer is what a fake signer prints.
type answer struct {
	out  string
	code int
	err  error
}

type fakeSigner struct {
	ans   answer
	calls []signing.StdioCall
}

func (s *fakeSigner) RunStdio(_ context.Context, c signing.StdioCall) ([]byte, int, error) {
	s.calls = append(s.calls, c)
	return []byte(s.ans.out), s.ans.code, s.ans.err
}

func clientFor(f *fixture, s *fakeSigner) *signing.Client {
	return &signing.Client{Runner: s, Src: f.src, Exe: "/bin/nself-ci", Env: []string{"NSELF_CI_HOME=/h"}}
}

func envsJSON(envs ...sdk.Envelope) string {
	b, _ := json.Marshal(signing.Response{Envelopes: envs})
	return "noise " + string(b)
}

func TestClientSignRoundTrip(t *testing.T) {
	f := newFixture(t)
	items := []signing.Item{item(signing.PayloadStatement, f.statementPayload(nil))}
	good, err := f.handler().Handle(context.Background(), request(items...), decodeAll(t, request(items...)))
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSigner{ans: answer{out: envsJSON(good...)}}
	c := clientFor(f, s)
	c.Timeout = 7 * time.Second
	got, err := c.Sign(context.Background(), runID, items)
	if err != nil || len(got) != 1 {
		t.Fatal(err)
	}
	call := s.calls[0]
	if strings.Join(call.Argv, " ") != "/bin/nself-ci artifacts signer --stdio" || call.Timeout != 7*time.Second || call.Env[0] != "NSELF_V15=1" || call.Env[1] != "NSELF_CI_HOME=/h" {
		t.Fatalf("%+v", call)
	}
	var req signing.Request
	if json.Unmarshal(call.Stdin, &req) != nil || req.RunID != runID || len(req.Items) != 1 {
		t.Fatalf("request %s", call.Stdin)
	}
	c.Timeout = 0
	if _, err := c.Sign(context.Background(), runID, items); err != nil || s.calls[1].Timeout != signing.DefaultTimeout {
		t.Fatalf("default timeout %v %v", s.calls[1].Timeout, err)
	}
}

func TestClientRefusesBeforeStartingTheSignerWhileAnAttemptRuns(t *testing.T) {
	f := newFixture(t)
	f.src.attempts[0].State = "running"
	s := &fakeSigner{}
	_, err := clientFor(f, s).Sign(context.Background(), runID, []signing.Item{item(signing.PayloadStatement, []byte("x"))})
	if !signing.IsReason(err, signing.ReasonPayloadRejected) || len(s.calls) != 0 {
		t.Fatalf("%v (%d signer starts)", err, len(s.calls))
	}
}

func TestClientFailsClosed(t *testing.T) {
	f := newFixture(t)
	items := []signing.Item{item(signing.PayloadStatement, f.statementPayload(nil))}
	good, _ := f.handler().Handle(context.Background(), request(items...), decodeAll(t, request(items...)))
	two := good[0]
	two.Signatures = append(two.Signatures, two.Signatures[0])
	wrongPayload := good[0]
	wrongPayload.Payload = b64([]byte("other"))
	wrongType := good[0]
	wrongType.PayloadType = "text/plain"
	badSig := good[0]
	badSig.Signatures = []sdk.EnvelopeSig{{KeyID: good[0].Signatures[0].KeyID, Sig: "!!"}}
	noSig := good[0]
	noSig.Signatures = nil
	refusal := `{"error":{"code":"E717","reason":"digest_not_committed","message":"artifact x"}}`
	cases := map[string]struct {
		ans    answer
		reason string
	}{
		"signer crashed":      {answer{err: errors.New("killed")}, signing.ReasonSignerFailed},
		"timed out":           {answer{err: context.DeadlineExceeded}, signing.ReasonSignerFailed},
		"exit 2 no output":    {answer{code: 2}, signing.ReasonSignerFailed},
		"refusal passes on":   {answer{out: refusal, code: 2}, signing.ReasonDigestNotCommitted},
		"refusal with exit 0": {answer{out: refusal}, signing.ReasonSignerFailed},
		"refusal no reason":   {answer{out: `{"error":{"code":"E717"}}`, code: 2}, signing.ReasonSignerFailed},
		"no output":           {answer{}, signing.ReasonSignerFailed},
		"garbage":             {answer{out: "{not json"}, signing.ReasonSignerFailed},
		"no envelopes":        {answer{out: `{}`}, signing.ReasonSignerFailed},
		"too many envelopes":  {answer{out: envsJSON(good[0], good[0])}, signing.ReasonSignerFailed},
		"two signatures":      {answer{out: envsJSON(two)}, signing.ReasonSignerFailed},
		"no signature":        {answer{out: envsJSON(noSig)}, signing.ReasonSignerFailed},
		"payload substituted": {answer{out: envsJSON(wrongPayload)}, signing.ReasonSignerFailed},
		"type substituted":    {answer{out: envsJSON(wrongType)}, signing.ReasonSignerFailed},
		"malformed signature": {answer{out: envsJSON(badSig)}, signing.ReasonSignerFailed},
	}
	for name, c := range cases {
		got, err := clientFor(f, &fakeSigner{ans: c.ans}).Sign(context.Background(), runID, items)
		if err == nil || got != nil || reasonOf(t, err) != c.reason {
			t.Errorf("%s: %v %v", name, got, err)
		}
	}
	if _, err := clientFor(f, &fakeSigner{ans: answer{out: refusal, code: 2}}).Sign(context.Background(), runID, items); !strings.Contains(err.Error(), "artifact x") {
		t.Fatalf("the signer's reason text was lost: %v", err)
	}
}

func TestClientRefusesOversizeRequest(t *testing.T) {
	f := newFixture(t)
	s := &fakeSigner{}
	big := signing.Item{PayloadType: signing.PayloadStatement, PayloadB64: strings.Repeat("A", signing.MaxRequestBytes)}
	if _, err := clientFor(f, s).Sign(context.Background(), runID, []signing.Item{big}); !signing.IsReason(err, signing.ReasonPayloadRejected) || len(s.calls) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestClientKeyCalls(t *testing.T) {
	f := newFixture(t)
	info := `{"key_id":"ci-release-0123456789abcdef","public_key":"AAAA","old_key_id":"ci-release-fedcba9876543210"}`
	for mode, call := range map[string]func(*signing.Client) (signing.KeyInfo, error){
		"--public": func(c *signing.Client) (signing.KeyInfo, error) { return c.PublicKey(context.Background()) },
		"--init":   func(c *signing.Client) (signing.KeyInfo, error) { return c.Init(context.Background()) },
		"--rotate": func(c *signing.Client) (signing.KeyInfo, error) { return c.Rotate(context.Background()) },
	} {
		s := &fakeSigner{ans: answer{out: info + "\n"}}
		got, err := call(clientFor(f, s))
		if err != nil || got.KeyID != "ci-release-0123456789abcdef" || got.Public != "AAAA" || got.OldKeyID != "ci-release-fedcba9876543210" {
			t.Fatalf("%s: %+v %v", mode, got, err)
		}
		if strings.Join(s.calls[0].Argv, " ") != "/bin/nself-ci artifacts signer "+mode || s.calls[0].Stdin != nil {
			t.Fatalf("%s: %+v", mode, s.calls[0])
		}
		for _, bad := range []answer{{out: `{"public_key":"A"}`}, {out: `{"key_id":"k"}`}, {out: "junk"}, {out: ""}, {err: errors.New("x")}, {out: `{"error":{"code":"E717","reason":"key_missing","message":"no key"}}`, code: 2}} {
			if _, err := call(clientFor(f, &fakeSigner{ans: bad})); err == nil {
				t.Fatalf("%s: accepted %+v", mode, bad)
			}
		}
	}
	_, err := clientFor(f, &fakeSigner{ans: answer{out: `{"error":{"code":"E717","reason":"key_missing","message":"no key"}}`, code: 2}}).PublicKey(context.Background())
	if !signing.IsReason(err, signing.ReasonKeyMissing) {
		t.Fatalf("%v", err)
	}
}

func TestClientLocatesItsOwnBinary(t *testing.T) {
	f := newFixture(t)
	s := &fakeSigner{ans: answer{out: `{"key_id":"k","public_key":"p"}`}}
	c := &signing.Client{Runner: s, Src: f.src}
	if _, err := c.PublicKey(context.Background()); err != nil || s.calls[0].Argv[0] == "" || s.calls[0].Argv[1] != "artifacts" {
		t.Fatalf("%v %v", err, s.calls)
	}
}
