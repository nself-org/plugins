//go:build security

package redteam

// Purpose: TM-33, TM-29, TM-43 red team of the credentialed provider commands (`providers test|show|list`,
// `forgejo`, `doctor --providers`). Attacks: a provider API URL spelled to reach loopback, link-local or
// private space (connect-time SSRF guard), a proxy variable, the credential value in any output channel
// (text, JSON, stderr, store, argv of gh), a pasted token in place of a variable name, and the Forgejo
// probe pointed at another host with --url.
// Inputs:  the nself-ci binary (see f_attack_helpers_test.go), loopback canaries, fakeSecret.
// Outputs: pass = the control held; fail = the attack worked.
// Constraints: loopback only. Private and link-local targets are refused at connect time by the guard
//          under test, so no packet leaves the machine; with a broken guard a connect would be attempted.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fTokenVar = "NSELF_REDTEAM_TOKEN"

func fSet(t *testing.T, b *fBin, key, value string) {
	t.Helper()
	if out, errOut, exit := b.run(nil, "policy", "set", "--scope", "user", key, value); exit != 0 {
		t.Fatalf("policy set %s %s: %s %s", key, value, out, errOut)
	}
}

// TestF08ProbeURLsNeverReachLocalOrPrivateSpace (TM-33): the provider API URL is spelled as loopback in
// many ways, as link-local, RFC1918, CGNAT and ULA, and a proxy variable points at a canary; the canary sees
// no connection and the token never leaves.
func TestF08ProbeURLsNeverReachLocalOrPrivateSpace(t *testing.T) {
	b := fNew(t)
	v4 := newFCanary(t, "tcp4", "127.0.0.1:0")
	if v4 == nil {
		t.Fatal("no loopback listener")
	}
	v6 := newFCanary(t, "tcp6", "[::1]:0")
	token := "ghp_" + fakeSecret
	b.extra = []string{fTokenVar + "=" + token}
	fSet(t, b, "providers.github.token_env", `"`+fTokenVar+`"`)
	hosts := []string{"127.0.0.1:" + v4.port(), "localhost:" + v4.port(), "127.1:" + v4.port(), "2130706433:" + v4.port(), "0x7f000001:" + v4.port(),
		"017700000001:" + v4.port(), "[::ffff:127.0.0.1]:" + v4.port(), "localhost.:" + v4.port(), "127.0.0.1.:" + v4.port(), "0.0.0.0:" + v4.port(),
		"169.254.169.254", "10.0.0.5", "192.168.1.1", "100.64.0.1", "[fd00::1]", "[fe80::1]"}
	if v6 != nil {
		hosts = append(hosts, "[::1]:"+v6.port(), "[0:0:0:0:0:ffff:7f00:1]:"+v6.port())
	}
	for _, h := range hosts {
		fSet(t, b, "providers.github.api_url", `"https://`+h+`"`)
		out, errOut, _ := b.run(nil, "providers", "test", "github", "--json")
		if strings.Contains(out+errOut, fakeSecret) {
			t.Errorf("host %s: the token is in the output", h)
		}
	}
	// positive control: with the operator opt-in the same canary is reached, so the zero above is the guard
	ctl := newFCanary(t, "tcp4", "127.0.0.1:0")
	fSet(t, b, "providers.github.allow_private_base", `true`)
	fSet(t, b, "providers.github.api_url", `"https://127.0.0.1:`+ctl.port()+`"`)
	b.run(nil, "providers", "test", "github")
	if ctl.hits.Load() == 0 {
		t.Error("control: allow_private_base did not reach the canary, so the zero hits above prove nothing")
	}
	fSet(t, b, "providers.github.allow_private_base", `false`)
	// a proxy variable must not make the funnel dial a proxy on loopback either
	fSet(t, b, "providers.github.api_url", `"https://api.github.com"`)
	b.run([]string{"HTTPS_PROXY=http://127.0.0.1:" + v4.port(), "https_proxy=http://127.0.0.1:" + v4.port(), "ALL_PROXY=http://127.0.0.1:" + v4.port()}, "providers", "test", "github")
	if n := v4.hits.Load(); n != 0 {
		t.Errorf("the IPv4 canary accepted %d connection(s); it saw %q", n, v4.seen())
	}
	if v6 != nil && v6.hits.Load() != 0 {
		t.Errorf("the IPv6 canary accepted %d connection(s)", v6.hits.Load())
	}
}

// TestF09CredentialNeverPrinted (TM-29, TM-43): a GitHub token in a variable, in gh's output and a GitLab
// token and runner token and webhook secret in variables never appear in any command output, the doctor,
// the state database, or the argv of gh.
func TestF09CredentialNeverPrinted(t *testing.T) {
	b := fNew(t)
	b.extra = []string{fTokenVar + "=github_pat_" + fakeSecret, "NSELF_REDTEAM_GL=glpat-" + fakeSecret, "NSELF_REDTEAM_GLR=glrt-" + fakeSecret, "NSELF_REDTEAM_WH=whsec-" + fakeSecret}
	fSet(t, b, "providers.github.token_env", `"`+fTokenVar+`"`)
	fSet(t, b, "providers.gitlab.token_env", `"NSELF_REDTEAM_GL"`)
	fSet(t, b, "providers.gitlab.runner_token_env", `"NSELF_REDTEAM_GLR"`)
	var all strings.Builder
	grab := func(args ...string) {
		out, errOut, _ := b.run(nil, args...)
		all.WriteString(out + errOut)
	}
	for _, id := range []string{"github", "github-hosted", "gitlab"} {
		grab("providers", "show", id)
		grab("providers", "show", id, "--json", "--repo", "o/r")
		grab("providers", "test", id, "--repo", "o/r")
		grab("providers", "test", id, "--json", "--repo", "o/r")
		grab("providers", "authorize", id, "--project", b.repo, "--json")
		grab("providers", "revoke", id, "--project", b.repo)
	}
	grab("providers", "list")
	grab("providers", "list", "--json")
	grab("doctor", "--providers", "--json")
	grab("doctor", "--providers")
	// the same with gh as the credential source (its unmasked output holds the token)
	fSet(t, b, "providers.github.token_env", `""`)
	grab("providers", "show", "github", "--json")
	grab("providers", "test", "github")
	grab("doctor", "--providers", "--json")
	if !strings.Contains(all.String(), "env:"+fTokenVar) || !strings.Contains(all.String(), "fine-grained") {
		t.Error("control: the output does not show the credential source and kind, so the scan below proves nothing")
	}
	if strings.Contains(all.String(), fakeSecret) {
		i := strings.Index(all.String(), fakeSecret)
		t.Errorf("a credential reached command output: ...%s...", all.String()[max(0, i-60):min(len(all.String()), i+60)])
	}
	if raw, err := os.ReadFile(filepath.Join(b.home, "state.db")); err == nil && strings.Contains(string(raw), fakeSecret) {
		t.Error("a credential reached the state database")
	}
	if raw, err := os.ReadFile(b.ghLog); err == nil && strings.Contains(string(raw), fakeSecret) {
		t.Errorf("a credential reached the argv of gh: %s", raw)
	}
}

// TestF10PastedTokenAsVariableName (TM-29): an operator who pastes the token instead of the variable name
// into providers.github.token_env must be refused; otherwise the token is stored, written to the audit row
// and printed by list, show and the doctor as the credential "source".
func TestF10PastedTokenAsVariableName(t *testing.T) {
	if !openFinding(t, "P03") {
		return
	}
	b := fNew(t)
	token := "ghp_" + strings.Repeat("A1b2C3d4E5", 3) + "xyz012"
	if _, _, exit := b.run(nil, "policy", "set", "--scope", "user", "providers.github.token_env", token); exit == 0 {
		out, _, _ := b.run(nil, "providers", "list", "--json")
		doc := b.doctor(nil)
		t.Errorf("a pasted token was accepted as a variable name; list prints it: %v; doctor DP001: %v", strings.Contains(out, token), doc["CI-DP001"]["message"])
	}
}

// TestF11ForgejoURLFlagCannotRedirectTheAdminPassword (TM-33, TM-43): the admin password goes to the
// policy-resolved host only. A machine-invoked caller passing --url must not make it go to another host.
func TestF11ForgejoURLFlagCannotRedirectTheAdminPassword(t *testing.T) {
	if !openFinding(t, "P04") {
		return
	}
	b := fNew(t)
	srv := newFHTTP(t, `{"healthy":true}`)
	if srv == nil {
		t.Fatal("no loopback listener")
	}
	machine := []string{"NSELF_INVOKED_BY=mcp", "NSELF_FORGEJO_ADMIN_USER=admin", "NSELF_FORGEJO_ADMIN_PASSWORD=" + fakeSecret}
	out, errOut, _ := b.run(machine, "forgejo", "--url", srv.URL)
	if got := srv.auth(); len(got) != 0 {
		t.Errorf("the admin password reached %s as %q (policy URL is not set; output %q %q)", srv.URL, got, out, errOut)
	}
	if strings.Contains(out+errOut, fakeSecret) {
		t.Error("the admin password is in the output")
	}
}
