// Purpose: build the environment helm runs with.
//
// Inputs: the parent environment (os.Environ()).
//
// Outputs: the child environment: the parent's, minus every NSELF_* variable
// (NSELF_PLUGIN_LICENSE_KEY and other nSelf secrets) and with HELM_DEBUG forced
// to false.
//
// Constraints: helm reads HELM_DEBUG and then prints the user-supplied values,
// computed values and rendered manifest (every secret of the stack) on install
// and upgrade. A debug variable set in a shell or CI must not reach that.
// KUBECONFIG, PATH, HOME and the other HELM_* settings pass through.
package k8s

import "strings"

// helmEnv returns parent without NSELF_* and HELM_DEBUG, plus HELM_DEBUG=false.
func helmEnv(parent []string) []string {
	out := make([]string, 0, len(parent)+1)
	for _, kv := range parent {
		if strings.HasPrefix(kv, "NSELF_") || strings.HasPrefix(kv, "HELM_DEBUG=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HELM_DEBUG=false")
}
