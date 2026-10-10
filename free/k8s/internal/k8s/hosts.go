// Purpose: tell the user what an install reaches and which install-time values
// the embedded chart does not consume yet (D-0311).
//
// Inputs: the generated values.yaml (ingress.rules[].host) and InstallOptions.
//
// Outputs: IngressHosts (sorted, unique hosts routed by the chart) and
// NotPassed (names of the install-time values an upgrade will not pass).
//
// Constraints: the hosts come from the generated ingress.rules, never from
// --domain: the chart does not read domain, license.key or plugins.install
// yet, so --domain is recorded in the release and changes nothing else.
package k8s

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// IngressHosts returns the sorted, unique hosts of ingress.rules in the
// generated values.yaml of the project in dir. No rule means no host (nil).
func IngressHosts(dir string) ([]string, error) {
	vf, err := ResolveValues(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(vf.Values)
	if err != nil {
		return nil, fmt.Errorf("k8s: read values: %w", err)
	}
	var doc struct {
		Ingress struct {
			Rules []struct {
				Host string `yaml:"host"`
			} `yaml:"rules"`
		} `yaml:"ingress"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("k8s: parse values: %w", err)
	}
	seen := map[string]bool{}
	var hosts []string
	for _, r := range doc.Ingress.Rules {
		if r.Host != "" && !seen[r.Host] {
			seen[r.Host] = true
			hosts = append(hosts, r.Host)
		}
	}
	sort.Strings(hosts)
	return hosts, nil
}

// HasUnusedLicence reports that a licence key is set although it is not passed
// to helm: the chart does not read license.key yet (D-0311), and helm would
// store the key in every release revision Secret.
func HasUnusedLicence(opts InstallOptions) bool { return opts.LicenseKey != "" }

// NotPassed names the install-time values that opts leaves empty. An upgrade
// applies values afresh, so these are not passed to helm. The licence key is
// never passed (HasUnusedLicence), so it is not listed here.
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
