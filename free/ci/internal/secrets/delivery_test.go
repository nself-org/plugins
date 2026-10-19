package secrets

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestCarrierValueAllowedFullProduct(t *testing.T) {
	modes := []trust.CoordinatorMode{trust.Personal, trust.Team, trust.CoordinatorMode(""), trust.CoordinatorMode("other")}
	carriers := []string{"ssh", "wss", "https", ""}
	owners := []string{"operator", "other", "team", ""}
	classes := append(model.EnumValues("SecretClass"), "bogus", "")
	trues := 0
	for _, m := range modes {
		for _, c := range carriers {
			for _, o := range owners {
				for _, k := range classes {
					got := CarrierValueAllowed(m, c, o, model.SecretClass(k))
					want := m == trust.Personal && c == "ssh" && o == "operator" && (k == "project" || k == "environment")
					if got != want {
						t.Fatalf("(%v,%q,%q,%q) = %v", m, c, o, k, got)
					}
					if got {
						trues++
					}
				}
			}
		}
	}
	if trues != 2 {
		t.Fatalf("%d allowed combinations, want 2", trues)
	}
}

func TestContainerArgsNeverCarryValues(t *testing.T) {
	vals := map[string]string{"FOO": planted, "BAR": "other-fake-value-2"}
	args, err := ContainerArgs(vals)
	if err != nil || strings.Join(args, " ") != "-e BAR -e FOO" {
		t.Fatalf("%v %v", args, err)
	}
	env, err := ContainerCLIEnv(vals)
	if err != nil || len(env) != 2 || env[1] != "FOO="+planted {
		t.Fatalf("%v %v", env, err)
	}
	pe, _ := ProcessEnv(vals)
	if strings.Join(pe, ";") != "BAR=other-fake-value-2;FOO="+planted {
		t.Fatalf("%v", pe)
	}
	if _, err = ContainerArgs(map[string]string{"bad name": "x"}); CodeOf(err) != "E674" {
		t.Fatalf("bad name accepted: %v", err)
	}
}

func TestTartStdinQuotesAndRefusesNUL(t *testing.T) {
	b, err := TartStdin(map[string]string{"FOO": "it's $(x)"})
	if err != nil || string(b) != "export FOO='it'\\''s $(x)'\n" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err = TartStdin(map[string]string{"FOO": "a\x00b"}); CodeOf(err) != "E674" || strings.Contains(err.Error(), "a\x00b") {
		t.Fatalf("%v", err)
	}
}

func legacyFake() *fakeNself { return newFake(map[string]string{}) }

func TestLegacyEnvOnOperatorsOwnPersonalNode(t *testing.T) {
	const name = "ANDROID_KEYSTORE_PASSWORD"
	legacy := &LegacyEnv{Getenv: func(k string) string {
		if k == name {
			return "legacy-fake-pass-99"
		}
		return ""
	}}
	var events []Event
	f := legacyFake()
	s, rec := newSrc(f, func(c *Config) {
		c.Legacy = legacy
		c.Audit = func(e Event) error { events = append(events, e); return nil }
	})
	got, err := s.Resolve(context.Background(), []string{"release/" + name})
	if err != nil || got[name] != "legacy-fake-pass-99" || rec.count() != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if s.Sources()["release/"+name] != LegacySource || len(events) != 1 || events[0].Kind != EventLegacyEnv || events[0].Ref != "release/"+name {
		t.Fatalf("sources %v events %v", s.Sources(), events)
	}
	if strings.Contains(strings.Join([]string{events[0].Kind, events[0].Attempt, events[0].Ref, events[0].Source}, " "), "legacy-fake-pass-99") {
		t.Fatal("value in an audit event")
	}

	// a remote node never uses the legacy source, even with the variable set
	remote, _ := newSrc(legacyFake(), func(c *Config) {
		c.Mode, c.NodeEligible, c.NodeID, c.AgeKeyPath, c.Legacy = ModeAgentNode, true, "n1", "/node/age.key", legacy
	})
	if got, err = remote.Resolve(context.Background(), []string{"release/" + name}); got != nil || CodeOf(err) != "E676" {
		t.Fatalf("remote node used the legacy source: %v %v", got, err)
	}
	// not the operator's own node
	other, _ := newSrc(legacyFake(), func(c *Config) {
		c.Legacy = legacy
		c.Facts.Runner.PersonalOwnNode = false
	})
	if got, err = other.Resolve(context.Background(), []string{"release/" + name}); got != nil || err == nil {
		t.Fatalf("non-own node used the legacy source: %v %v", got, err)
	}
	// a name outside the four, or a class other than release, is never covered
	for _, ref := range []string{"project/" + name, "release/OTHER_NAME"} {
		if (LegacyEnv{Getenv: legacy.Getenv}).Covers(mustParse(t, ref)) {
			t.Fatalf("%s is covered", ref)
		}
	}
	// the variable is unset: the cli's miss stands
	empty, _ := newSrc(legacyFake(), func(c *Config) { c.Legacy = &LegacyEnv{Getenv: func(string) string { return "" }} })
	if _, err = empty.Resolve(context.Background(), []string{"release/" + name}); CodeOf(err) != "E676" {
		t.Fatalf("%v", err)
	}
}

func TestLegacyEnvStoreValueWinsAndMixedRefs(t *testing.T) {
	const name = "ANDROID_KEYSTORE_PASSWORD"
	f := newFake(map[string]string{"project/FOO": planted})
	legacy := &LegacyEnv{Getenv: func(string) string { return "legacy-fake-pass-99" }}
	n := 0
	s, _ := newSrc(f, func(c *Config) { c.Legacy = legacy; c.Audit = func(Event) error { n++; return nil } })
	got, err := s.Resolve(context.Background(), []string{"project/FOO", "release/" + name})
	if err != nil || got["FOO"] != planted || got[name] != "legacy-fake-pass-99" || n != 1 {
		t.Fatalf("%v %v audits=%d", got, err, n)
	}
	// a ref that is in the store never consults the environment
	f.store["release/"+name] = "store-fake-pass-0001"
	n = 0
	s2, _ := newSrc(f, func(c *Config) { c.Legacy = legacy; c.Audit = func(Event) error { n++; return nil } })
	got, err = s2.Resolve(context.Background(), []string{"release/" + name})
	if err != nil || got[name] != "store-fake-pass-0001" || n != 0 {
		t.Fatalf("%v %v", got, err)
	}
}

func mustParse(t *testing.T, s string) Ref {
	t.Helper()
	r, err := Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFailedResolveHoldsNothingAndLeaksNoValue(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	s, rec := newSrc(f, nil)
	_, err := s.Resolve(context.Background(), []string{"project/FOO", "project/MISSING"})
	if CodeOf(err) != "E676" || s.Held() != 0 || rec.count() != 0 || strings.Contains(err.Error(), planted) {
		t.Fatalf("%v held=%d", err, s.Held())
	}
	f.errAll = "E999 with spaces"
	_, err = s.Resolve(context.Background(), []string{"project/FOO"})
	if CodeOf(err) != "E676" || strings.Contains(err.Error(), "spaces") {
		t.Fatalf("unbounded cli text echoed: %v", err)
	}
}

func TestRecipientsCall(t *testing.T) {
	f := newFake(nil)
	rcp := "age1" + strings.Repeat("q", 58)
	if err := Recipients(context.Background(), f, "", "/p", "add", "release", rcp, "release"); err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(f.Calls()[0].Argv, " ")
	if argv != "nself config secrets recipients add --file release --project /p --allow-release-class "+rcp {
		t.Fatalf("%q", argv)
	}
	for _, bad := range []string{"", "--flag", "age1UPPER", "x"} {
		if err := Recipients(context.Background(), f, "", "/p", "add", "ci", bad, "project"); CodeOf(err) != "E674" {
			t.Fatalf("%q accepted", bad)
		}
	}
	if err := Recipients(context.Background(), f, "", "/p", "wipe", "ci", rcp, "project"); CodeOf(err) != "E674" {
		t.Fatal("bad verb accepted")
	}
	if len(f.Calls()) != 1 {
		t.Fatal("a bad call reached the cli")
	}
}

func TestCancelledFirstProbeDoesNotPoisonLaterAttempts(t *testing.T) {
	f := newFake(map[string]string{"project/FOO": planted})
	shared := &ProbeState{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s1, _ := newSrc(f, func(c *Config) { c.Resolver = &Resolver{Runner: f, Project: "/p", Shared: shared} })
	_, _ = s1.Resolve(ctx, []string{"project/FOO"})
	s2, _ := newSrc(f, func(c *Config) { c.Resolver = &Resolver{Runner: f, Project: "/p", Shared: shared} })
	if got, err := s2.Resolve(context.Background(), []string{"project/FOO"}); err != nil || got["FOO"] != planted {
		t.Fatalf("a cancelled first attempt poisoned the probe: %v %v", got, err)
	}
}

func TestPastedSecretIsNotEchoedInRefError(t *testing.T) {
	for _, bad := range []string{"ghp_notarealtokenvalue_fake_0123456789", "project/not a name with secret-fake-77", "xyz/ABC"} {
		_, err := Parse(bad)
		if CodeOf(err) != "E674" || strings.Contains(err.Error(), "fake") || strings.Contains(err.Error(), "ghp_") {
			t.Fatalf("%q echoed: %v", bad, err)
		}
	}
}

func TestLegacyEnvNotUsedWhenAuditFails(t *testing.T) {
	const name = "ANDROID_KEYSTORE_PASSWORD"
	legacy := &LegacyEnv{Getenv: func(string) string { return "legacy-fake-pass-99" }}
	s, rec := newSrc(legacyFake(), func(c *Config) {
		c.Legacy = legacy
		c.Audit = func(Event) error { return fmt.Errorf("disk full") }
	})
	got, err := s.Resolve(context.Background(), []string{"release/" + name})
	if got != nil || CodeOf(err) != "E676" || s.Held() != 0 || rec.count() != 0 || strings.Contains(err.Error(), "legacy-fake") {
		t.Fatalf("%v %v", got, err)
	}
	// and without any audit sink the compat source is off
	s2, _ := newSrc(legacyFake(), func(c *Config) { c.Legacy = legacy; c.Audit = nil })
	if _, err = s2.Resolve(context.Background(), []string{"release/" + name}); CodeOf(err) != "E676" {
		t.Fatalf("%v", err)
	}
}
