// Purpose: assemble the helm argv for install and upgrade.
//
// Inputs: the verb, InstallOptions, the extracted chart directory, the
// generated values files and the overrides file ("" when none).
//
// Outputs: the argument list after the helm binary name.
//
// Constraints: only paths and non-secret flags go on argv. Values come from
// --values files in this order: generated values.yaml, generated secrets.yaml,
// install overrides (later files win). Never --set or --set-string.
package k8s

// buildArgs returns the helm argument list for verb ("install" or "upgrade").
func buildArgs(verb string, opts InstallOptions, chartDir string, vf ValuesFiles, overrides string) []string {
	release := opts.ReleaseName
	if release == "" {
		release = HelmReleaseName
	}
	args := []string{verb, release, chartDir, "--values", vf.Values, "--values", vf.Secrets}
	if overrides != "" {
		args = append(args, "--values", overrides)
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
