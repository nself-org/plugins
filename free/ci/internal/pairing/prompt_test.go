package pairing

// Purpose: the PromptApprover on its own (scripted bufio reader): exact-fingerprint approval, defaults,
// list parsing, denial paths, EOF, and one prompt at a time.
// Inputs:  scripted answers. Outputs: pass/fail. Constraints: no terminal, no goroutine without a bound.

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

const csrFP = "SHA256:abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"

func promptReq(name string) ApprovalRequest {
	return ApprovalRequest{OfferID: "offer-1", Label: "lab", NodeID: name, NodeName: name, CSRFingerprint: csrFP, NodeKeyID: "deadbeefdeadbeef",
		OS: "linux", Arch: "arm64", SourceAddr: "192.0.2.7", CertTTL: 30 * time.Minute}
}

func scripted(script string) (*PromptApprover, *syncBuf) {
	out := &syncBuf{}
	return &PromptApprover{In: bufio.NewReader(strings.NewReader(script)), Out: out}, out
}

func TestPromptApprover(t *testing.T) {
	ctx := context.Background()
	t.Run("the exact fingerprint then Enter keeps the defaults", func(t *testing.T) {
		p, out := scripted(csrFP + "\n\n\n\n\n")
		d, err := p.Approve(ctx, promptReq("node-p"))
		if err != nil || !d.Approve {
			t.Fatalf("%+v %v", d, err)
		}
		def := DefaultTrust()
		if strings.Join(d.Trust.Accepts, ",") != strings.Join(def.Accepts, ",") || strings.Join(d.Trust.SecretClasses, ",") != "none" || d.Trust.PrivacyZone != "" || len(d.Trust.Projects) != 0 {
			t.Fatalf("trust %+v", d.Trust)
		}
		for _, want := range []string{"node-p", "linux/arm64", "192.0.2.7", csrFP, "deadbeefdeadbeef", "30m0s", "lab", "[owner,internal]", "[none]"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("the screen lacks %q:\n%s", want, out.String())
			}
		}
		if p.Kind() != "admin" {
			t.Fatalf("kind %q", p.Kind())
		}
	})

	t.Run("typed lists are split on commas and trimmed; surrounding blanks around the fingerprint are fine", func(t *testing.T) {
		p, _ := scripted("  " + csrFP + "  \n owner , collaborator ,, \n none,team\nlocal-only\nproj-1 ,proj-2,\n")
		d, err := p.Approve(ctx, promptReq("node-p"))
		if err != nil || !d.Approve {
			t.Fatalf("%+v %v", d, err)
		}
		if strings.Join(d.Trust.Accepts, "|") != "owner|collaborator" || strings.Join(d.Trust.SecretClasses, "|") != "none|team" ||
			d.Trust.PrivacyZone != "local-only" || strings.Join(d.Trust.Projects, "|") != "proj-1|proj-2" {
			t.Fatalf("trust %+v", d.Trust)
		}
	})

	t.Run("anything but the exact fingerprint is a denial that asks nothing more", func(t *testing.T) {
		for _, typed := range []string{"", "yes", "y", "SHA256:wrong", strings.ToLower(csrFP), csrFP + "x", csrFP[:len(csrFP)-1], strings.TrimPrefix(csrFP, "SHA256:")} {
			p, out := scripted(typed + "\nowner\nnone\nlocal-only\nproj\n")
			d, err := p.Approve(ctx, promptReq("node-p"))
			if err != nil || d.Approve || d.Reason == "" {
				t.Fatalf("%q: %+v %v", typed, d, err)
			}
			if strings.Contains(out.String(), "Trust classes") {
				t.Fatalf("%q: trust asked after a failed confirmation", typed)
			}
		}
	})

	t.Run("end of input is an error, never an approval", func(t *testing.T) {
		for _, script := range []string{"", csrFP + "\n", csrFP + "\nowner\n", csrFP + "\nowner\nnone\nlocal-only\n"} {
			p, _ := scripted(script)
			d, err := p.Approve(ctx, promptReq("node-p"))
			if err == nil || d.Approve || !errors.Is(err, io.EOF) {
				t.Fatalf("%q: %+v %v", script, d, err)
			}
		}
	})

	t.Run("a last line without a newline is still read", func(t *testing.T) {
		p, _ := scripted(csrFP + "\n\n\n\nproj-9")
		d, err := p.Approve(ctx, promptReq("node-p"))
		if err != nil || !d.Approve || len(d.Trust.Projects) != 1 || d.Trust.Projects[0] != "proj-9" {
			t.Fatalf("%+v %v", d, err)
		}
	})

	t.Run("one prompt at a time: a second redemption waits for the first answer", func(t *testing.T) {
		admin := newAdminScript(t)
		type res struct {
			d   Decision
			err error
		}
		first, second := make(chan res, 1), make(chan res, 1)
		go func() { d, err := admin.ap.Approve(ctx, promptReq("node-1")); first <- res{d, err} }()
		eventually(t, 2*time.Second, "the first prompt is shown", func() bool { return strings.Count(admin.screen(), "A node asks to join") == 1 })
		go func() { d, err := admin.ap.Approve(ctx, promptReq("node-2")); second <- res{d, err} }()
		time.Sleep(150 * time.Millisecond)
		if n := strings.Count(admin.screen(), "A node asks to join"); n != 1 || strings.Contains(admin.screen(), "node-2") {
			t.Fatalf("the second question interleaved:\n%s", admin.screen())
		}
		go func() { _, _ = admin.pw.Write([]byte(csrFP + "\n\n\n\n\n")) }()
		if r := <-first; r.err != nil || !r.d.Approve {
			t.Fatalf("first: %+v", r)
		}
		eventually(t, 2*time.Second, "the second prompt appears after the first finished", func() bool { return strings.Count(admin.screen(), "A node asks to join") == 2 })
		go func() { _, _ = admin.pw.Write([]byte("\n")) }()
		if r := <-second; r.err != nil || r.d.Approve {
			t.Fatalf("second: %+v", r)
		}
	})

	t.Run("DefaultTrust is ADR 0025: owner and internal, no secrets, and callers cannot change it", func(t *testing.T) {
		d := DefaultTrust()
		d.Accepts[0], d.SecretClasses[0] = "collaborator", "deploy"
		again := DefaultTrust()
		if strings.Join(again.Accepts, ",") != "owner,internal" || strings.Join(again.SecretClasses, ",") != "none" {
			t.Fatalf("%+v", again)
		}
	})

	t.Run("an existing node keeps its trust: no trust questions, a notice, and the Decision carries no fields", func(t *testing.T) {
		p, out := scripted(csrFP + "\nowner\nnone\nlocal-only\nproj\n")
		req := promptReq("node-p")
		req.Existing = true
		d, err := p.Approve(ctx, req)
		if err != nil || !d.Approve || len(d.Trust.Accepts) != 0 || len(d.Trust.Projects) != 0 || d.Trust.PrivacyZone != "" {
			t.Fatalf("%+v %v", d, err)
		}
		if strings.Contains(out.String(), "Trust classes") || !strings.Contains(out.String(), "already registered") {
			t.Fatalf("screen:\n%s", out.String())
		}
		wrong, _ := scripted("nope\n")
		if d, err = wrong.Approve(ctx, req); err != nil || d.Approve {
			t.Fatalf("an existing node still needs the exact fingerprint: %+v %v", d, err)
		}
	})

	t.Run("cancelling ctx while waiting for the admin's line returns ctx.Err() promptly", func(t *testing.T) {
		admin := newAdminScript(t)
		cctx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { _, err := admin.ap.Approve(cctx, promptReq("node-p")); done <- err }()
		eventually(t, 60*time.Second, "the prompt is shown", func() bool { return strings.Contains(admin.screen(), "Type the CSR key fingerprint") })
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Approve did not return after cancel")
		}
	})

	t.Run("a second Approve waits for its turn and gives up with its own ctx", func(t *testing.T) {
		admin := newAdminScript(t)
		first := make(chan error, 1)
		go func() { _, err := admin.ap.Approve(ctx, promptReq("node-1")); first <- err }()
		eventually(t, 60*time.Second, "the first prompt is shown", func() bool { return strings.Contains(admin.screen(), "node-1") })
		cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := admin.ap.Approve(cctx, promptReq("node-2"))
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
			t.Fatalf("err %v after %s", err, time.Since(start))
		}
		if strings.Contains(admin.screen(), "node-2") {
			t.Fatal("the waiting redemption printed its question")
		}
		_ = admin.pw.Close() // end the first prompt
		if err = <-first; err == nil {
			t.Fatal("the first prompt should fail on end of input")
		}
	})

	t.Run("control characters in name, os, arch, source and label never reach the terminal", func(t *testing.T) {
		p, out := scripted("\n")
		req := promptReq("evil\x1b[2J\rname")
		req.OS, req.Arch, req.SourceAddr, req.Label = "li\x07nux", "ar\x1bch", "1.2.3.4\x00", "la\nbel\x1b]0;x\x07"
		if _, err := p.Approve(ctx, req); err != nil {
			t.Fatal(err)
		}
		for _, b := range []byte(out.String()) {
			if b < 0x20 && b != '\n' || b == 0x7f {
				t.Fatalf("control byte %#x in the prompt:\n%q", b, out.String())
			}
		}
		if !strings.Contains(out.String(), "evil [2J name") {
			t.Fatalf("name not shown sanitized: %q", out.String())
		}
		long := promptReq(strings.Repeat("n", 500))
		pl, outl := scripted("\n")
		_, _ = pl.Approve(ctx, long)
		if strings.Contains(outl.String(), strings.Repeat("n", 100)) {
			t.Fatal("a 500 character name was printed in full")
		}
	})
}
