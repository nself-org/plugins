// Purpose: assemble the helm argv for install and upgrade.
//
// Inputs: the verb, InstallOptions, the extracted chart directory, the
// generated values files and whether an overlay is piped on stdin.
//
// Outputs: the argument list after the helm binary name.
//
// Constraints: only paths and non-secret flags go on argv. Values come from
// --values files in this order: generated values.yaml, generated secrets.yaml,
// then the install overlay read from stdin as "--values -" (later files win).
// Never --set or --set-string.
package k8s

// buildArgs returns the helm argument list for verb ("install" or "upgrade").
func buildArgs(verb string, opts InstallOptions, chartDir string, vf ValuesFiles, overlay bool) []string {
	release := opts.ReleaseName
	if release == "" {
		release = HelmReleaseName
	}
	args := []string{verb, release, chartDir, "--values", vf.Values, "--values", vf.Secrets}
	if overlay {
		args = append(args, "--values", "-")
	}
	if opts.Wait {
		timeout := opts.Timeout
		if timeout == "" {
			timeout = DefaultTimeout
		}
		args = append(args, "--wait", "--timeout", timeout)
	}
	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}
	return args
}
