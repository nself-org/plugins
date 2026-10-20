package tart

// Purpose: map a network scope to the softnet flags `tart run` gets, with block rules from netscope.
// Inputs:  the scope, the netscope Facts (on-link prefixes, host addresses, deploy exclusion set) and the
//          Tart NAT gateway address.
// Outputs: a softnetPlan: the extra `tart run` argv, the block and allow lists, and whether a proxy is needed.
// Constraints: none, restricted and internet run under softnet with default deny: 0.0.0.0/0 is blocked and
//          only the gateway /32 is allowed (the ssh session and the proxy live there); the exclusion set,
//          on-link prefixes and host addresses are listed as explicit blocks too, and the gateway is cut out of
//          every block so the rules hold whichever way softnet orders allow against block. lan and privileged
//          use Tart's default NAT (owner/internal work only; netscope refuses them for untrusted jobs).
//          IPv6 prefixes are not softnet rules and are ignored. No pf, no root.

import (
	"net/netip"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

// maxSoftnetRules bounds the block list so the tart argv stays small.
const maxSoftnetRules = 512

// softnetPlan is what `tart run` is started with for one scope.
type softnetPlan struct {
	Scope model.NetworkScope
	// Args are the softnet flags (empty for lan and privileged).
	Args         []string
	Block, Allow []netip.Prefix
	// NeedProxy is true for restricted and internet: the VM reaches the world only through the host proxy.
	NeedProxy bool
}

func effectiveScope(s model.NetworkScope) model.NetworkScope {
	if s == "" {
		return "none"
	}
	return s
}

// planSoftnet builds the plan. softnetBlock is the probed fact that the installed Tart has the block option.
func planSoftnet(scope model.NetworkScope, f netscope.Facts, gateway netip.Addr, softnetBlock bool) (softnetPlan, error) {
	scope = effectiveScope(scope)
	switch scope {
	case "lan", "privileged":
		return softnetPlan{Scope: scope}, nil
	case "none", "restricted", "internet":
	default:
		return softnetPlan{}, isolation.ScopeUnenforceable("network scope %q is not known to the Tart profile", scope)
	}
	if !softnetBlock {
		return softnetPlan{}, isolation.ScopeUnenforceable("the installed Tart has no softnet allow/block option, so the %s scope cannot be enforced", scope)
	}
	if !gateway.Is4() {
		return softnetPlan{}, isolation.ScopeUnenforceable("the Tart gateway %q is not an IPv4 address", gateway)
	}
	block := []netip.Prefix{netip.PrefixFrom(netip.IPv4Unspecified(), 0)}
	block = append(block, f.Exclusion...)
	block = append(block, f.OnLink...)
	for _, a := range f.HostAddrs {
		a = netscope.Unmap(a)
		block = append(block, netip.PrefixFrom(a, a.BitLen()))
	}
	var rules []netip.Prefix
	for _, p := range block {
		if !p.Addr().Is4() {
			continue
		}
		rules = append(rules, subtractAddr(p.Masked(), gateway)...)
	}
	rules = dedupe(rules)
	if len(rules) > maxSoftnetRules {
		return softnetPlan{}, isolation.ScopeUnenforceable("%d block rules exceed the limit of %d", len(rules), maxSoftnetRules)
	}
	allow := []netip.Prefix{netip.PrefixFrom(gateway, 32)}
	return softnetPlan{
		Scope: scope, Block: rules, Allow: allow, NeedProxy: scope != "none",
		Args: []string{"--net-softnet", "--net-softnet-block=" + joinPrefixes(rules), "--net-softnet-allow=" + joinPrefixes(allow)},
	}, nil
}

// subtractAddr returns the prefixes that cover p except the single address a (p itself when it excludes a).
func subtractAddr(p netip.Prefix, a netip.Addr) []netip.Prefix {
	if !p.Contains(a) {
		return []netip.Prefix{p}
	}
	var out []netip.Prefix
	for bits := p.Bits(); bits < a.BitLen(); bits++ {
		child := netip.PrefixFrom(a, bits+1).Masked()
		sibling := flipBit(child, bits)
		out = append(out, sibling)
	}
	return out
}

// flipBit returns the prefix with the same length as child whose last bit differs: the half of the parent
// that does not hold the carved-out address.
func flipBit(child netip.Prefix, parentBits int) netip.Prefix {
	b := child.Addr().As4()
	idx := parentBits
	b[idx/8] ^= 1 << (7 - uint(idx%8))
	return netip.PrefixFrom(netip.AddrFrom4(b), child.Bits()).Masked()
}

func dedupe(in []netip.Prefix) []netip.Prefix {
	seen := map[netip.Prefix]bool{}
	var out []netip.Prefix
	for _, p := range in {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if c := out[i].Addr().Compare(out[j].Addr()); c != 0 {
			return c < 0
		}
		return out[i].Bits() < out[j].Bits()
	})
	return out
}

func joinPrefixes(ps []netip.Prefix) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = p.String()
	}
	return strings.Join(s, ",")
}
