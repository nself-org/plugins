package netscope

import (
	"bytes"
	"net/netip"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestEffectiveScope(t *testing.T) {
	for class, want := range map[model.TrustClass]model.NetworkScope{"owner": "lan", "internal": "lan", "collaborator": "internet", "untrusted": "restricted"} {
		if got, err := Effective(class, ""); err != nil || got != want {
			t.Errorf("%s default: %q %v, want %s", class, got, err, want)
		}
	}
	if _, err := Effective("untrusted", "lan"); err == nil || !strings.Contains(err.Error(), CodeScopeFailed) {
		t.Errorf("untrusted lan: %v", err)
	}
	if _, err := Effective("untrusted", "privileged"); err == nil {
		t.Error("untrusted privileged accepted")
	}
	if got, err := Effective("untrusted", "none"); err != nil || got != "none" {
		t.Errorf("untrusted none: %q %v", got, err)
	}
	if got, err := Effective("owner", "privileged"); err != nil || got != "privileged" {
		t.Errorf("owner privileged: %q %v", got, err)
	}
	if _, err := Effective("owner", "weird"); err == nil {
		t.Error("unknown scope accepted")
	}
}

func TestNoLANRouteIsDerivedFromTheProof(t *testing.T) {
	enforced := []model.NetworkScope{"none", "restricted"}
	for _, c := range []struct {
		scope model.NetworkScope
		want  bool
	}{{"none", true}, {"restricted", true}, {"internet", false}, {"lan", false}, {"privileged", false}, {"", false}} {
		if got := NoLANRoute(c.scope, enforced); got != c.want {
			t.Errorf("%q: %v, want %v", c.scope, got, c.want)
		}
	}
	if NoLANRoute("restricted", nil) {
		t.Error("no proof, still no LAN route")
	}
	// lan and privileged are never in the enforceable list, so a runner cannot prove them away
	for _, s := range Enforceable() {
		if s == "lan" || s == "privileged" {
			t.Errorf("%s is enforceable", s)
		}
	}
	d := trust.Declaration{NoLANRoute: true}
	Apply(&d, "internet", enforced)
	if d.NoLANRoute {
		t.Error("Apply kept a declared NoLANRoute for a scope without proof")
	}
}

func TestConfigRoundTripAndRefusals(t *testing.T) {
	f := Facts{OnLink: []netip.Prefix{netip.MustParsePrefix("10.1.0.0/16"), netip.MustParsePrefix("2a00::/64")},
		HostAddrs: []netip.Addr{netip.MustParseAddr("10.1.0.5")}, Exclusion: []netip.Prefix{netip.MustParsePrefix("5.75.235.42/32")}, ExclusionHosts: []string{"prod.example.com"}}
	raw, err := NewConfig("restricted", "a1", netip.MustParsePrefix("172.30.0.0/24"), []string{"x.example.org"}, f).Encode()
	if err != nil {
		t.Fatal(err)
	}
	c, got, err := DecodeConfig(raw)
	if err != nil || c.Scope != "restricted" || len(got.OnLink) != 2 || len(got.HostAddrs) != 1 || len(got.Exclusion) != 1 || len(got.ExclusionHosts) != 1 {
		t.Fatalf("%+v %+v %v", c, got, err)
	}
	if strings.ContainsAny(raw, "\n\r") {
		t.Error("encoded config holds a newline (it travels in an env file)")
	}
	for name, bad := range map[string]string{
		"not json": "{", "scope none": `{"scope":"none","job_prefix":"172.30.0.0/24"}`, "scope lan": `{"scope":"lan","job_prefix":"172.30.0.0/24"}`,
		"bad prefix": `{"scope":"internet","job_prefix":"nope"}`, "bad on-link": `{"scope":"internet","job_prefix":"172.30.0.0/24","on_link":["x"]}`,
		"bad host": `{"scope":"internet","job_prefix":"172.30.0.0/24","host_addrs":["x"]}`, "bad exclusion": `{"scope":"internet","job_prefix":"172.30.0.0/24","exclusion":["1.2.3.4"]}`,
	} {
		if _, _, err := DecodeConfig(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	huge := Config{Scope: "internet", JobPrefix: "172.30.0.0/24", Hosts: []string{strings.Repeat("h", maxConfigBytes)}}
	if _, err := huge.Encode(); err == nil {
		t.Error("oversized config encoded (a deny list would be truncated)")
	}
}

func TestEventsRoundTripAndIgnoreJunk(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(ReadyLine())
	buf.Write(Event{Scope: "internet", Host: "10.0.0.1", Port: "443", Reason: ReasonSpecial}.Encode())
	buf.WriteString("not json\n{\"kind\":\"other\"}\n")
	buf.Write(Event{Scope: "internet", Host: "x", Reason: ReasonNotAllowed}.Encode())
	if !HasReady(buf.Bytes()) {
		t.Error("ready line not found")
	}
	got := ParseEvents(&buf)
	if len(got) != 2 || got[0].Host != "10.0.0.1" || got[1].Reason != ReasonNotAllowed || got[0].At == "" {
		t.Fatalf("%+v", got)
	}
	// a hostile name cannot forge the readiness line: the quotes are escaped inside the event
	forged := Event{Scope: "internet", Host: `"kind":"network.ready"`, Reason: ReasonNotAllowed}.Encode()
	if HasReady(forged) {
		t.Error("a denied host name forged the readiness line")
	}
}

func TestHostMatching(t *testing.T) {
	if !HostAllowed("restricted", "Proxy.Golang.Org.", nil) || HostAllowed("restricted", "evilproxy.golang.org", nil) || HostAllowed("restricted", "10.0.0.1", nil) {
		t.Error("restricted allowlist")
	}
	if HostAllowed("restricted", "sub.proxy.golang.org", nil) {
		t.Error("subdomains of an allowlisted host are not allowlisted")
	}
	if !HostAllowed("internet", "anything.example", nil) || HostAllowed("none", "proxy.golang.org", nil) || HostAllowed("lan", "proxy.golang.org", nil) {
		t.Error("scope gate")
	}
	if NormalizeHost("a b") != "" || NormalizeHost("a/b") != "" || NormalizeHost("u@h") != "" {
		t.Error("host normalisation accepts separators")
	}
	if !hostExcluded([]string{"example.com"}, "api.example.com") || hostExcluded([]string{"example.com"}, "notexample.com") {
		t.Error("exclusion host matching")
	}
}
