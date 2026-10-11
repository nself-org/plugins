// Purpose: find the project's BASE_DOMAIN in the resolved compose model, so the
// chart can rewrite the generated ingress hosts to the --domain of an install
// (debt D-0311).
//
// Inputs: the resolved Model.
//
// Outputs: the base domain, or "" when the model has no reliable value.
//
// Constraints: compose has already interpolated BASE_DOMAIN into the env of the
// services that use it (nginx and the custom services). The nginx value wins;
// otherwise every service that sets the variable must agree. A value that still
// holds "${" or is empty counts as absent. A conflict yields "": nothing is
// guessed, and the chart then leaves the hosts as generated.
package values

import "strings"

const baseDomainEnv = "BASE_DOMAIN"

// baseDomain returns the project's BASE_DOMAIN, or "" when none is reliable.
func baseDomain(m *Model) string {
	if d := domainOf(m.Services["nginx"]); d != "" {
		return d
	}
	found := ""
	for _, name := range sortedKeys(m.Services) {
		d := domainOf(m.Services[name])
		switch {
		case d == "":
		case found == "":
			found = d
		case found != d:
			return ""
		}
	}
	return found
}

func domainOf(cs ComposeSvc) string {
	v := cs.Environment[baseDomainEnv]
	if v == nil || strings.Contains(*v, "${") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*v))
}
