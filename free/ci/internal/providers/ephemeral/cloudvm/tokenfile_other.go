//go:build !unix

package cloudvm

// Purpose: the non-unix half of the token file check: file modes cannot be trusted there, so the
// cloud VM provider refuses to start (E698 unsupported) and the coordinator never reads a token.

func readTokenFile(string) (string, error) {
	return "", fail(ReasonUnsupported, nil, "the cloud VM provider needs a unix coordinator to keep the token file private")
}
