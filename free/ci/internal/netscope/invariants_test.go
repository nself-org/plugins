package netscope

// Purpose: the two invariants this package owns (Epic D11): I07 (untrusted never gets LAN) and I18 (untrusted
// cannot reach the deploy network). Both run over every enumerated world and 10000 generated ones through
// internal/invariants, and print the INVARIANT line the verify step greps.

import (
	"fmt"
	"math/rand"
	"net/netip"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestInvariant_I07(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		if trust.DecodeTrust(string(w.Trust)) != "untrusted" {
			return nil
		}
		eff, err := Effective(w.Trust, "")
		if err != nil || eff != "restricted" {
			return fmt.Errorf("an untrusted job without a declared scope got %q (%v), want restricted", eff, err)
		}
		eff, err = Effective(w.Trust, w.Network)
		lan := w.Network == "lan" || w.Network == "privileged"
		switch {
		case lan && err == nil:
			return fmt.Errorf("an untrusted job declaring %s was not refused (got %q)", w.Network, eff)
		case !lan && (err != nil || eff != w.Network):
			return fmt.Errorf("an untrusted job declaring %s got %q (%v)", w.Network, eff, err)
		case eff == "lan" || eff == "privileged":
			return fmt.Errorf("an untrusted job's effective scope is %s", eff)
		}
		return nil
	})
}

// i18World draws the facts of one world from rnd: on-link prefixes, host addresses, an exclusion set.
func i18World(rnd *rand.Rand) Facts {
	var f Facts
	for i := 0; i < 1+rnd.Intn(3); i++ {
		a := randomAddr(rnd)
		bits := 8 + rnd.Intn(17)
		if a.Is6() {
			bits = 32 + rnd.Intn(33)
		}
		p, _ := a.Prefix(bits)
		f.OnLink = append(f.OnLink, p)
		f.HostAddrs = append(f.HostAddrs, a)
	}
	for i := 0; i < rnd.Intn(4); i++ {
		a := randomAddr(rnd)
		if rnd.Intn(2) == 0 {
			f.Exclusion = append(f.Exclusion, netip.PrefixFrom(a, a.BitLen()))
		} else {
			p, _ := a.Prefix(a.BitLen() - rnd.Intn(9))
			f.Exclusion = append(f.Exclusion, p)
		}
	}
	return f
}

func randomAddr(rnd *rand.Rand) netip.Addr {
	if rnd.Intn(3) == 0 {
		var b [16]byte
		rnd.Read(b[:])
		b[0] = 0x20 + byte(rnd.Intn(0x20)) // global unicast range 2000::/3
		return netip.AddrFrom16(b)
	}
	return netip.AddrFrom4([4]byte{byte(1 + rnd.Intn(223)), byte(rnd.Intn(256)), byte(rnd.Intn(256)), byte(1 + rnd.Intn(254))})
}

// forms returns the spellings of a: itself, and for an IPv4 address its mapped, NAT64 and 6to4 forms.
func forms(a netip.Addr) []netip.Addr {
	out := []netip.Addr{a}
	if a.Is4() {
		b := a.As4()
		out = append(out,
			netip.AddrFrom16([16]byte{10: 0xff, 11: 0xff, 12: b[0], 13: b[1], 14: b[2], 15: b[3]}),
			netip.AddrFrom16([16]byte{0: 0x00, 1: 0x64, 2: 0xff, 3: 0x9b, 12: b[0], 13: b[1], 14: b[2], 15: b[3]}),
			netip.AddrFrom16([16]byte{0: 0x20, 1: 0x02, 2: b[0], 3: b[1], 4: b[2], 5: b[3], 15: 1}))
	}
	return out
}

// i18Oracle is the independent statement of the property: the unmapped address is in none of the sets.
func i18Oracle(a netip.Addr, f Facts) (string, bool) {
	a = a.Unmap()
	for _, c := range DenyTable() {
		if netip.MustParsePrefix(c).Contains(a) {
			return "special " + c, true
		}
	}
	for _, p := range append(append([]netip.Prefix(nil), f.OnLink...), f.Exclusion...) {
		if p.Contains(a) {
			return "set " + p.String(), true
		}
	}
	for _, h := range f.HostAddrs {
		if h.Unmap() == a {
			return "host", true
		}
	}
	return "", false
}

func TestInvariant_I18(t *testing.T) {
	rnd := rand.New(rand.NewSource(invariants.Seed()))
	decl := trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged",
		UIDSeparation: true, Ownership: "operator"}
	enforcedSets := [][]model.NetworkScope{nil, {"none"}, {"none", "restricted"}, {"none", "restricted", "internet"}, {"restricted"}}
	admitted, withheld := 0, 0
	invariants.Check(t, func(w invariants.World) error {
		if trust.DecodeTrust(string(w.Trust)) != "untrusted" {
			return nil
		}
		f := i18World(rnd)
		var cands []netip.Addr
		for _, p := range f.OnLink {
			cands = append(cands, forms(p.Addr())...)
		}
		for _, p := range f.Exclusion {
			cands = append(cands, forms(p.Addr())...)
		}
		cands = append(cands, f.HostAddrs...)
		for i := 0; i < 3; i++ {
			cands = append(cands, forms(randomAddr(rnd))...)
		}
		for _, a := range cands {
			if v := Allowed(w.Network, a, f); v.Allowed {
				if why, bad := i18Oracle(a, f); bad {
					return fmt.Errorf("scope %s allowed %s which is in %s", w.Network, a, why)
				}
				if w.Network != "restricted" && w.Network != "internet" {
					return fmt.Errorf("scope %s allowed %s without a proxy", w.Network, a)
				}
			}
		}
		// the runner side: an untrusted job is admitted only where its scope was proven
		enforced := enforcedSets[rnd.Intn(len(enforcedSets))]
		eff, err := Effective(w.Trust, w.Network)
		if err != nil {
			return nil // refused before placement
		}
		d := decl
		Apply(&d, eff, enforced)
		reasons := trust.Admissible(trust.JobFacts{Trust: w.Trust, Network: eff, Mode: trust.DecodeCoordinatorMode(w.Mode)}, d, trust.Effective{})
		proven := false
		for _, e := range enforced {
			proven = proven || e == eff
		}
		if len(reasons) == 0 {
			admitted++
			if !proven {
				return fmt.Errorf("an untrusted %s job was admitted to a runner enforcing %v", eff, enforced)
			}
		} else if !proven {
			withheld++
		}
		return nil
	})
	if admitted == 0 || withheld == 0 {
		t.Fatalf("the runner side was not exercised: admitted=%d withheld=%d", admitted, withheld)
	}
	t.Logf("untrusted admitted on a proving runner %d times, withheld %d times", admitted, withheld)
}
