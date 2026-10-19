package toolchain

// Purpose: the executables the pinned toolchain image provides, so a plan made
// for `--container` places jobs against the image and not the host PATH.
// Inputs:  the embedded versions.yaml (tool rows and their version probes).
// Outputs: sorted executable names.
// Constraints: a tool row contributes the executable its probe runs; git and
// npm come with the image's base stage and node. There is no Swift or Xcode
// row, so neither is listed (Epic D33): a Swift job needs its own
// container.image.

import "sort"

// imageExtras are executables the Dockerfile installs outside a tool row.
var imageExtras = []string{"git", "npm"}

// ImageTools lists the executables inside the pinned toolchain image.
func ImageTools() ([]string, error) {
	v, err := Embedded()
	if err != nil {
		return nil, err
	}
	out := append([]string(nil), imageExtras...)
	for _, r := range v.Rows {
		if r.Kind == KindTool && len(r.Probe) > 0 {
			out = append(out, r.Probe[0])
		}
	}
	sort.Strings(out)
	return out, nil
}
