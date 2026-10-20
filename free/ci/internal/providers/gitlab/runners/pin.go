package runners

// Purpose: PinnedImage, the gitlab-runner image reference for a job, read from the embedded toolchain table.
// Inputs:  the gitlab-runner-image row of toolchain/versions.yaml (generated from
//          toolchain/pins/gitlab-runner-image.yaml); NSELF_CI_TOOLCHAIN_VERSIONS is not consulted.
// Outputs: repository@sha256:<index digest>, the same string the images.d fragment pins.
// Constraints: a row that is missing, pending or not a digest reference is an error: no tag fallback.

import (
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// PinRow is the name of the pin row.
const PinRow = "gitlab-runner-image"

// PinnedImage returns the digest-pinned runner image.
func PinnedImage() (string, error) {
	v, err := toolchain.Embedded()
	if err != nil {
		return "", err
	}
	row, ok := v.Row(PinRow)
	if !ok || row.Status != toolchain.StatusPinned || row.Image == "" || row.Digest == "" {
		return "", errors.New("runners: the gitlab-runner image is not pinned")
	}
	return row.Image + "@" + row.Digest, nil
}
