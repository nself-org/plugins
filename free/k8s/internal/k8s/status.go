// Purpose: `helm status` reduced to a safe summary.
//
// Inputs: release name and optional kubeconfig.
//
// Outputs: one JSON line with name, namespace, version (the release revision),
// chart_version and status.
//
// Constraints: helm's own JSON carries the release config (every value of
// values.yaml, secrets.yaml and the overlay) and the rendered manifest (the
// Secret objects). None of that is decoded or printed: only the five fields
// above survive. Output that is not JSON is an error that does not echo it.
package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ReleaseSummary is what `nself k8s status` prints.
type ReleaseSummary struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Version      int    `json:"version"`
	ChartVersion string `json:"chart_version"`
	Status       string `json:"status"`
}

// helmRelease is the subset of helm's release JSON that Status decodes.
type helmRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Chart     struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	} `json:"chart"`
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
}

// ErrStatusUnparsable is returned when helm status prints something that is
// not a release JSON document. The output is never echoed.
var ErrStatusUnparsable = errors.New("k8s: helm status: output is not a release JSON document")

// Status returns the release summary as a JSON line.
func Status(ctx context.Context, releaseName, kubeconfig string) (string, error) {
	sum, err := ReleaseStatus(ctx, releaseName, kubeconfig)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(sum)
	if err != nil {
		return "", fmt.Errorf("k8s: helm status: %w", err)
	}
	return string(b), nil
}

// ReleaseStatus runs `helm status` and returns the five-field summary. A
// helm failure is a *HelmError; NotFound is set when helm's stderr says the
// release does not exist (the stderr text itself is dropped).
func ReleaseStatus(ctx context.Context, releaseName, kubeconfig string) (ReleaseSummary, error) {
	helm, err := helmBinary()
	if err != nil {
		return ReleaseSummary{}, err
	}
	if releaseName == "" {
		releaseName = HelmReleaseName
	}
	args := []string{"status", releaseName, "--output", "json"}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	cmd := exec.CommandContext(ctx, helm, args...)
	cmd.Env = helmEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		notFound := errors.As(err, &ee) && releaseMissing(ee.Stderr)
		return ReleaseSummary{}, &HelmError{Verb: "status", Err: err, NotFound: notFound,
			text: fmt.Sprintf("k8s: helm status: %v", err)}
	}
	var rel helmRelease
	if err := json.Unmarshal(out, &rel); err != nil {
		return ReleaseSummary{}, ErrStatusUnparsable
	}
	return ReleaseSummary{rel.Name, rel.Namespace, rel.Version, rel.Chart.Metadata.Version, rel.Info.Status}, nil
}

// releaseMissing reports whether helm's stderr says the release is not there.
func releaseMissing(stderr []byte) bool {
	return bytes.Contains(bytes.ToLower(stderr), []byte("not found"))
}
