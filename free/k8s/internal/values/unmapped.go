// Purpose: list the compose keys of a service that have no counterpart in the
// generated values, so nothing is dropped silently (Ticket risk: compose
// features without a k8s equivalent are listed, never approximated).
//
// Inputs: one resolved compose service.
//
// Outputs: sorted "key" or "key:detail" strings for Service.NotMapped.
//
// Constraints: only keys that change runtime behaviour are listed; logging,
// networks and container_name are ignored on purpose (the cluster owns them).
package values

import "strings"

func hostFeatures(cs ComposeSvc) []string {
	var out []string
	if cs.User != "" {
		out = append(out, "user:"+cs.User)
	}
	if cs.ReadOnly {
		out = append(out, "read_only")
	}
	if len(cs.CapAdd) > 0 {
		out = append(out, "cap_add:"+strings.Join(cs.CapAdd, ","))
	}
	if len(cs.CapDrop) > 0 {
		out = append(out, "cap_drop:"+strings.Join(cs.CapDrop, ","))
	}
	if len(cs.SecurityOpt) > 0 {
		out = append(out, "security_opt:"+strings.Join(cs.SecurityOpt, ","))
	}
	if present(cs.Deploy) {
		out = append(out, "deploy")
	}
	if present(cs.DependsOn) {
		out = append(out, "depends_on")
	}
	return out
}

func present(b []byte) bool {
	s := strings.TrimSpace(string(b))
	return s != "" && s != "null" && s != "{}" && s != "[]"
}
