package pairing

// Purpose: TestJoinFingerprint: the root-fingerprint step of join (no terminal, --expect-fingerprint,
// interactive confirm) and the admin's confirmation of the CSR fingerprint with the PromptApprover,
// driven by a scripted admin that types what join printed.
// Inputs:  a rig. Outputs: pass/fail. Constraints: nothing leaves the node before the root fingerprint is
//          confirmed; issuance only after the exact CSR fingerprint is typed.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
	"github.com/nself-org/plugins/free/ci/internal/pki"
)

func TestJoinFingerprint(t *testing.T) {
	t.Run("no terminal and no flag refuses with E660 fingerprint_unconfirmed before anything is sent", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:j", DefaultTrust())})
		real := r.serveReal()
		out, asked := &syncBuf{}, false
		r.clock.Advance(2 * time.Second)
		opts, _ := r.joinOpts(real.URL, off.Code, "", "node-j", out)
		opts.Code = func() (string, error) { asked = true; return off.Code, nil }
		_, err := runJoin(opts)
		wantCoded(t, err, "E660", pki.ReasonFingerprintUnconfirmed)
		if asked || real.Cnt.hellos.Load() != 0 || real.Cnt.redeem.Load() != 0 || strings.Contains(out.String(), "CSR key") {
			t.Fatalf("something left the node: asked %v out %q", asked, out.String())
		}
	})

	t.Run("--expect-fingerprint equal to the served root enrolls without a terminal", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:j", DefaultTrust())
		off := r.offer(OfferOptions{Approver: ap})
		real := r.serveReal()
		res, err := r.join(real.URL, off.Code, off.RootFingerprint, "node-j")
		if err != nil || res.NodeID != "node-j" || len(ap.calls()) != 1 {
			t.Fatalf("%+v %v", res, err)
		}
	})

	t.Run("a different --expect-fingerprint refuses with E660 fingerprint_mismatch before any CSR is sent", func(t *testing.T) {
		r := newRig(t, nil)
		ap := approving("policy:j", DefaultTrust())
		off := r.offer(OfferOptions{Approver: ap})
		real := r.serveReal()
		other := newRig(t, nil).ca.RootFingerprint()
		flipped := off.RootFingerprint[:len(off.RootFingerprint)-1] + "A"
		if flipped == off.RootFingerprint {
			flipped = off.RootFingerprint[:len(off.RootFingerprint)-1] + "B"
		}
		for _, bad := range []string{other, flipped, strings.ToLower(off.RootFingerprint), off.RootFingerprint + " ", strings.TrimPrefix(off.RootFingerprint, "SHA256:")} {
			out := &syncBuf{}
			r.clock.Advance(2 * time.Second)
			opts, _ := r.joinOpts(real.URL, off.Code, bad, "node-j", out)
			_, err := runJoin(opts)
			wantCoded(t, err, "E660", pki.ReasonFingerprintMismatch)
			if strings.Contains(out.String(), "CSR key") || strings.Contains(err.Error(), off.Code) {
				t.Fatalf("output %q err %v", out.String(), err)
			}
		}
		if real.Cnt.hellos.Load() != 0 || real.Cnt.redeem.Load() != 0 || len(ap.calls()) != 0 {
			t.Fatal("the listener saw a request")
		}
	})

	t.Run("an interactive confirm decides when there is no flag; a refusal or an error is unconfirmed", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:j", DefaultTrust())})
		real := r.serveReal()
		var shown string
		for i, answer := range []func(string) (bool, error){
			func(fp string) (bool, error) { shown = fp; return false, nil },
			func(fp string) (bool, error) { shown = fp; return false, errors.New("EOF") },
		} {
			r.clock.Advance(2 * time.Second)
			opts, _ := r.joinOpts(real.URL, off.Code, "", "node-j", &syncBuf{})
			opts.Confirm = answer
			_, err := runJoin(opts)
			wantCoded(t, err, "E660", pki.ReasonFingerprintUnconfirmed)
			if shown != off.RootFingerprint {
				t.Fatalf("case %d showed %q", i, shown)
			}
		}
		if real.Cnt.hellos.Load() != 0 {
			t.Fatal("a request was sent before confirmation")
		}
		r.clock.Advance(2 * time.Second)
		opts, _ := r.joinOpts(real.URL, off.Code, "", "node-j", &syncBuf{})
		opts.Confirm = func(fp string) (bool, error) { return fp == off.RootFingerprint, nil }
		if res, err := runJoin(opts); err != nil || res.RootFingerprint != off.RootFingerprint {
			t.Fatalf("confirmed join: %+v %v", res, err)
		}
	})

	t.Run("join prints its CSR key fingerprint and issuance waits for the admin to type exactly that fingerprint and the trust fields", scriptedAdminApproves)
	t.Run("a wrong, empty or altered CSR fingerprint is denied: nothing is issued and no trust field is asked", scriptedAdminDenies)
	t.Run("the code is never printed or echoed by join", func(t *testing.T) {
		r := newRig(t, nil)
		off := r.offer(OfferOptions{Approver: approving("policy:j", DefaultTrust())})
		real := r.serveReal()
		out := &syncBuf{}
		r.clock.Advance(2 * time.Second)
		opts, _ := r.joinOpts(real.URL, off.Code, off.RootFingerprint, "node-j", out)
		if _, err := runJoin(opts); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), off.Code) || !strings.Contains(out.String(), "CSR key fingerprint: SHA256:") {
			t.Fatalf("join output %q", out.String())
		}
	})
	t.Run("the enroll URL must be https", func(t *testing.T) {
		r := newRig(t, nil)
		for _, u := range []string{"http://127.0.0.1:1/v1/enroll", "ftp://x", "://", "https:///v1/enroll", ""} {
			opts, _ := r.joinOpts(u, "AAAAAAAAAA", "", "node-j", &syncBuf{})
			if _, err := runJoin(opts); err == nil {
				t.Fatalf("%q accepted", u)
			}
		}
	})
}

// adminScript is an admin typing into the PromptApprover once join printed its CSR fingerprint.
type adminScript struct {
	ap *PromptApprover
	pw *io.PipeWriter
}

func newAdminScript(t *testing.T) *adminScript {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close(); _ = pr.Close() })
	return &adminScript{ap: &PromptApprover{In: bufio.NewReader(pr), Out: &syncBuf{}}, pw: pw}
}

func (a *adminScript) screen() string { return a.ap.Out.(*syncBuf).String() }

var csrLine = regexp.MustCompile(`CSR key fingerprint: (SHA256:\S+)`)

// typeAfter waits for join's output to show the CSR fingerprint, then types lines(fingerprint).
func (a *adminScript) typeAfter(joinOut *syncBuf, lines func(fp string) string) {
	go func() {
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			if m := csrLine.FindStringSubmatch(joinOut.String()); m != nil {
				_, _ = a.pw.Write([]byte(lines(m[1])))
				return
			}
		}
	}()
}

func scriptedAdminApproves(t *testing.T) {
	admin := newAdminScript(t)
	r := newRig(t, func(c *Config) { c.Approver = admin.ap })
	off := r.offer(OfferOptions{Label: "kitchen"}) // no Approver: the interactive default decides
	real := r.serveReal()
	out := &syncBuf{}
	admin.typeAfter(out, func(fp string) string { return fp + "\nowner,collaborator\nnone\nlocal-only\nproj-1, proj-2\n" })
	r.clock.Advance(2 * time.Second)
	opts, _ := r.joinOpts(real.URL, off.Code, off.RootFingerprint, "node-p", out)
	res, err := runJoin(opts)
	if err != nil {
		t.Fatalf("join: %v\nscreen: %s", err, admin.screen())
	}
	if !strings.Contains(admin.screen(), res.CSRFingerprint) || !strings.Contains(admin.screen(), "kitchen") || strings.Contains(admin.screen(), off.Code) {
		t.Fatalf("the prompt did not show the CSR fingerprint join printed (or showed the code): %s", admin.screen())
	}
	n, err := r.reg.Get(context.Background(), "node-p")
	if err != nil {
		t.Fatal(err)
	}
	tr := n.Capability.Trust
	if got := *tr.Accepts.Value; len(got) != 2 || got[0] != "owner" || got[1] != "collaborator" {
		t.Fatalf("accepts %v", got)
	}
	if got := *tr.SecretClasses.Value; len(got) != 1 || got[0] != "none" {
		t.Fatalf("secret classes %v", got)
	}
	if tr.PrivacyZone.Value == nil || *tr.PrivacyZone.Value != "local-only" {
		t.Fatalf("privacy zone %+v", tr.PrivacyZone)
	}
	if p := n.Capability.Lifecycle.AuthorizedProjects; len(p) != 2 || p[0] != "proj-1" || p[1] != "proj-2" {
		t.Fatalf("authorized projects %v", p)
	}
	rows := r.audit("pairing.approve")
	certs, _ := r.st.PKICertsOfNode(context.Background(), "node-p")
	if len(rows) != 1 || len(certs) != 1 {
		t.Fatalf("audit %+v certs %+v", rows, certs)
	}
	for _, want := range []string{"approver=admin", "node=node-p", "csr=" + res.CSRFingerprint, "serial=" + certs[0].Serial,
		"accepts=owner,collaborator", "secrets=none", "zone=local-only", "projects=proj-1,proj-2"} {
		if !strings.Contains(rows[0].Detail, want) {
			t.Fatalf("audit detail %q lacks %q", rows[0].Detail, want)
		}
	}
	r.noCodeInAudit(off.Code)
}

func scriptedAdminDenies(t *testing.T) {
	cases := []struct {
		name string
		typ  func(fp string) string
	}{
		{"wrong fingerprint", func(string) string { return "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n" }},
		{"empty line", func(string) string { return "\n" }},
		{"last character changed", func(fp string) string {
			if strings.HasSuffix(fp, "A") {
				return fp[:len(fp)-1] + "B\n"
			}
			return fp[:len(fp)-1] + "A\n"
		}},
		{"truncated", func(fp string) string { return fp[:len(fp)-4] + "\n" }},
		{"without the prefix", func(fp string) string { return strings.TrimPrefix(fp, "SHA256:") + "\n" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			admin := newAdminScript(t)
			r := newRig(t, func(c *Config) { c.Approver = admin.ap })
			off := r.offer(OfferOptions{})
			real := r.serveReal()
			out := &syncBuf{}
			admin.typeAfter(out, tc.typ)
			r.clock.Advance(2 * time.Second)
			opts, _ := r.joinOpts(real.URL, off.Code, off.RootFingerprint, "node-d", out)
			_, err := runJoin(opts)
			var re *enroll.RemoteError
			if !errors.As(err, &re) || re.Code != "E659" || re.Reason != ReasonDenied {
				t.Fatalf("want E659 denied, got %v", err)
			}
			if strings.Contains(admin.screen(), "Trust classes") {
				t.Fatal("the trust fields were asked after a failed confirmation")
			}
			if n := r.count(`SELECT count(*) FROM pki_cert`); n != 0 {
				t.Fatalf("%d certificates issued", n)
			}
			if _, gerr := r.reg.Get(context.Background(), "node-d"); gerr == nil {
				t.Fatal("the node was registered")
			}
			if len(r.audit("pairing.deny")) != 1 || len(r.audit("pairing.approve")) != 0 {
				t.Fatalf("audit %+v", r.audit(""))
			}
			r.noCodeInAudit(off.Code)
		})
	}
}
