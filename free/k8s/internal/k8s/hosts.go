// Purpose: tell the user what an install reaches and which install-time values
// an upgrade does not pass.
//
// Inputs: the generated values.yaml (baseDomain, ingress.rules[].host), the
// install --domain, and InstallOptions.
//
// Outputs: DomainHasNoEffect, IngressHosts (sorted, unique hosts the chart routes, with the domain
// rewrite applied), RewriteHost and NotPassed (names of the install-time values
// an upgrade will not pass).
//
// Constraints: RewriteHost is the Go twin of the chart helper nself.host
// (charts/nself/templates/_helpers.tpl): the success line must print the hosts
// the chart renders. Keep the two in step.
package k8s

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RewriteHost applies the install domain to one generated ingress host. When
// domain and baseDomain are both set and differ, the host equal to baseDomain
// becomes domain, and a host ending in "."+baseDomain keeps its prefix with
// the new domain (the prefix is lower-cased). Hosts are compared
// case-insensitively. Any other host is returned unchanged.
func RewriteHost(host, baseDomain, domain string) string {
	if domain == "" || baseDomain == "" || strings.EqualFold(domain, baseDomain) {
		return host
	}
	lh, lb := strings.ToLower(host), strings.ToLower(baseDomain)
	switch {
	case lh == lb:
		return domain
	case strings.HasSuffix(lh, "."+lb):
		return strings.TrimSuffix(lh, lb) + domain
	}
	return host
}

// hostsDoc is the part of the generated values.yaml the host rewrite reads.
type hostsDoc struct {
	BaseDomain string `yaml:"baseDomain"`
	Ingress    struct {
		Rules []struct {
			Host string `yaml:"host"`
		} `yaml:"rules"`
	} `yaml:"ingress"`
}

func readHostsDoc(dir string) (hostsDoc, error) {
	var doc hostsDoc
	vf, err := ResolveValues(dir)
	if err != nil {
		return doc, err
	}
	data, err := os.ReadFile(vf.Values)
	if err != nil {
		return doc, fmt.Errorf("k8s: read values: %w", err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return doc, fmt.Errorf("k8s: parse values: %w", err)
	}
	return doc, nil
}

// IngressHosts returns the sorted, unique hosts of ingress.rules in the
// generated values.yaml of the project in dir, after RewriteHost with the
// values' baseDomain and domain. No rule means no host (nil).
func IngressHosts(dir, domain string) ([]string, error) {
	doc, err := readHostsDoc(dir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var hosts []string
	for _, r := range doc.Ingress.Rules {
		h := RewriteHost(r.Host, doc.BaseDomain, domain)
		if r.Host != "" && !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	sort.Strings(hosts)
	return hosts, nil
}

// DomainHasNoEffect reports that domain is set but the generated values carry
// no baseDomain, so the chart has nothing to rewrite. An unreadable values file
// is not reported here (install and upgrade fail on it themselves).
func DomainHasNoEffect(dir, domain string) bool {
	if domain == "" {
		return false
	}
	doc, err := readHostsDoc(dir)
	return err == nil && doc.BaseDomain == ""
}

// DomainNoEffectHint is the warning for DomainHasNoEffect.
const DomainNoEffectHint = "--domain has no effect: the generated values have no baseDomain; run nself k8s values again"

// NotPassed names the install-time values that opts leaves empty. An upgrade
// applies values afresh, so these are not passed to helm. The licence Secret
// is not listed: LicenseKeyUnsent warns about it.
func NotPassed(opts InstallOptions) []string {
	var out []string
	if opts.Domain == "" {
		out = append(out, "domain")
	}
	if len(opts.Plugins) == 0 {
		out = append(out, "plugins")
	}
	return out
}
