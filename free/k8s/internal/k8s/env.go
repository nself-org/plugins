// Purpose: build the environment helm runs with.
//
// Inputs: the parent environment (os.Environ()).
//
// Outputs: the child environment: the parent's, minus every NSELF_* variable
// (NSELF_PLUGIN_LICENSE_KEY and other nSelf secrets) and with HELM_DEBUG forced
// to false. With an explicit kube context (helmEnvFor) it also drops every
// inherited HELM_KUBE* variable.
//
// Constraints: helm reads HELM_DEBUG and then prints the user-supplied values,
// computed values and rendered manifest (every secret of the stack) on install
// and upgrade. A debug variable set in a shell or CI must not reach that.
// KUBECONFIG, PATH, HOME and the other HELM_* settings pass through. helm
// reads HELM_KUBECONTEXT, HELM_KUBEAPISERVER and the auth overrides
// (HELM_KUBETOKEN, HELM_KUBEASUSER, HELM_KUBEASGROUPS, HELM_KUBECAFILE,
// HELM_KUBETLS_SERVER_NAME, HELM_KUBEINSECURE_SKIP_TLS_VERIFY) as defaults for
// its --kube-* flags, so when the caller names a context none may leak in
// (D-0376). Without one they all pass through.
package k8s

import "strings"

// helmEnv returns parent without NSELF_* and HELM_DEBUG, plus HELM_DEBUG=false.
func helmEnv(parent []string) []string { return helmEnvFor(parent, "") }

// helmEnvFor is helmEnv; when kubeContext is not empty it also drops
// every HELM_KUBE* variable so the explicit context wins.
func helmEnvFor(parent []string, kubeContext string) []string {
	out := make([]string, 0, len(parent)+1)
	for _, kv := range parent {
		if strings.HasPrefix(kv, "NSELF_") || strings.HasPrefix(kv, "HELM_DEBUG=") {
			continue
		}
		if kubeContext != "" && strings.HasPrefix(kv, "HELM_KUBE") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HELM_DEBUG=false")
}
