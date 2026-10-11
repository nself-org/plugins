// Purpose: build the environment helm runs with.
//
// Inputs: the parent environment (os.Environ()).
//
// Outputs: the child environment: the parent's, minus every NSELF_* variable
// (NSELF_PLUGIN_LICENSE_KEY and other nSelf secrets) and with HELM_DEBUG forced
// to false. With an explicit kube context (helmEnvFor) it also drops the
// inherited HELM_KUBECONTEXT and HELM_KUBEAPISERVER.
//
// Constraints: helm reads HELM_DEBUG and then prints the user-supplied values,
// computed values and rendered manifest (every secret of the stack) on install
// and upgrade. A debug variable set in a shell or CI must not reach that.
// KUBECONFIG, PATH, HOME and the other HELM_* settings pass through. helm
// reads HELM_KUBECONTEXT and HELM_KUBEAPISERVER itself, so when the caller
// names a context they must not override it (D-0376).
package k8s

import "strings"

// helmEnv returns parent without NSELF_* and HELM_DEBUG, plus HELM_DEBUG=false.
func helmEnv(parent []string) []string { return helmEnvFor(parent, "") }

// helmEnvFor is helmEnv; when kubeContext is not empty it also drops
// HELM_KUBECONTEXT and HELM_KUBEAPISERVER so the explicit context wins.
func helmEnvFor(parent []string, kubeContext string) []string {
	out := make([]string, 0, len(parent)+1)
	for _, kv := range parent {
		if strings.HasPrefix(kv, "NSELF_") || strings.HasPrefix(kv, "HELM_DEBUG=") {
			continue
		}
		if kubeContext != "" && (strings.HasPrefix(kv, "HELM_KUBECONTEXT=") || strings.HasPrefix(kv, "HELM_KUBEAPISERVER=")) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HELM_DEBUG=false")
}
