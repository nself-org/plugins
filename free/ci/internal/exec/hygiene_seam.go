package exec

// Purpose: the narrow seam internal/hygiene (P7-TRUST-09) needs from the one package that may start
// processes: list and remove the containers, networks and volumes labelled with an attempt id, and
// kill an attempt's process group by its marker.
// Inputs:  a runtime binary (docker or podman), a resource kind, an attempt id or resource id.
// Outputs: labelled resources (id plus the attempt label value); errors carry E613 (runtime) or E610.
// Constraints: only `ps`, `network ls`, `volume ls` and their `rm` forms run; ids and attempts are
// validated before they reach argv; removal is by id from a prior labelled listing, never by pattern.

import (
	"context"
	"io"
	osexec "os/exec"
	"regexp"
	"strings"
)

// AttemptLabel is the label every attempt-owned container, network and volume carries.
const AttemptLabel = "nself.ci.attempt"

// ResourceKind names a runtime resource type hygiene sweeps.
type ResourceKind string

// The resource kinds a runtime labels with the attempt id.
const (
	KindContainer ResourceKind = "container"
	KindNetwork   ResourceKind = "network"
	KindVolume    ResourceKind = "volume"
)

// Labelled is one runtime resource and the attempt label it carries.
type Labelled struct {
	ID      string
	Attempt string
	// Running is true for a container that is up (networks and volumes have no state).
	Running bool
}

var resourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// RuntimeListLabelled lists the resources of one kind that carry the attempt label. A non-empty
// attempt narrows the list to that attempt; an empty one returns every attempt-labelled resource.
func RuntimeListLabelled(ctx context.Context, binary string, kind ResourceKind, attempt string) ([]Labelled, error) {
	if !runtimeBinary.MatchString(binary) || (attempt != "" && !safeID.MatchString(attempt)) {
		return nil, coded("E613", "container runtime query refused")
	}
	filter := "label=" + AttemptLabel
	if attempt != "" {
		filter += "=" + attempt
	}
	label := `{{.Label "` + AttemptLabel + `"}}`
	var args []string
	switch kind {
	case KindContainer:
		args = []string{"ps", "-a", "--filter", filter, "--format", "{{.ID}} " + label + " {{.State}}"}
	case KindNetwork:
		args = []string{"network", "ls", "--filter", filter, "--format", "{{.ID}} " + label}
	case KindVolume:
		args = []string{"volume", "ls", "--filter", filter, "--format", "{{.Name}} " + label}
	default:
		return nil, coded("E613", "container runtime query refused")
	}
	out, err := runtimeOutput(ctx, binary, args...)
	if err != nil {
		return nil, err
	}
	var found []Labelled
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		want := 2
		if kind == KindContainer {
			want = 3 // id, attempt label, state
		}
		if len(f) != want || !resourceID.MatchString(f[0]) || !safeID.MatchString(f[1]) {
			// a labelled resource hygiene cannot name safely must not read as absent
			return nil, coded("E613", "container runtime listed a resource with an unsafe name or label")
		}
		if attempt != "" && f[1] != attempt {
			continue
		}
		found = append(found, Labelled{ID: f[0], Attempt: f[1], Running: want == 3 && f[2] == "running"})
	}
	return found, nil
}

// RuntimeRemoveLabelled removes one resource by the id a labelled listing returned, after asking the
// runtime once more that the resource still carries the attempt's label (a volume name can be
// recreated unlabelled between listing and removal). A resource that is already gone, or that no
// longer carries the label, is left alone and is not an error.
func RuntimeRemoveLabelled(ctx context.Context, binary string, kind ResourceKind, id, attempt string) error {
	if !runtimeBinary.MatchString(binary) || !resourceID.MatchString(id) || !safeID.MatchString(attempt) {
		return coded("E613", "container runtime removal refused")
	}
	var inspect []string
	switch kind {
	case KindContainer:
		inspect = []string{"inspect", "--format", `{{index .Config.Labels "` + AttemptLabel + `"}}`, id}
	case KindNetwork, KindVolume:
		inspect = []string{string(kind), "inspect", "--format", `{{index .Labels "` + AttemptLabel + `"}}`, id}
	default:
		return coded("E613", "container runtime removal refused")
	}
	ictx, icancel := context.WithTimeout(ctx, queryTimeout)
	defer icancel()
	icmd := runtimeCommand(ictx, binary, inspect)
	icmd.Env = dockerHostEnv()
	got, err := icmd.CombinedOutput()
	switch {
	case err != nil && goneText(string(got)):
		return nil
	case err != nil:
		return runtimeFailed(err)
	case strings.TrimSpace(string(got)) != attempt:
		return nil
	}
	var args []string
	switch kind {
	case KindContainer:
		args = []string{"rm", "-f", id}
	case KindNetwork:
		args = []string{"network", "rm", id}
	case KindVolume:
		args = []string{"volume", "rm", "-f", id}
	default:
		return coded("E613", "container runtime removal refused")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cmd := runtimeCommand(ctx, binary, args)
	cmd.Env = dockerHostEnv()
	out, err := cmd.CombinedOutput()
	if err != nil && !goneText(string(out)) {
		return runtimeFailed(err)
	}
	return nil
}

// goneText reports whether runtime output says the resource does not exist.
func goneText(out string) bool {
	out = strings.ToLower(out)
	return strings.Contains(out, "no such") || strings.Contains(out, "not found")
}

// runtimeOutput runs one fixed read-only listing and returns its capped stdout.
func runtimeOutput(ctx context.Context, binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	cmd := runtimeCommand(ctx, binary, args)
	cmd.Env = dockerHostEnv()
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, runtimeFailed(err)
	}
	if err := cmd.Start(); err != nil {
		return nil, runtimeFailed(err)
	}
	out, readErr := io.ReadAll(io.LimitReader(pipe, maxRuntimeOutput+1))
	_, _ = io.Copy(io.Discard, pipe)
	if err := cmd.Wait(); err != nil {
		return out, runtimeFailed(err)
	}
	if readErr != nil {
		return out, runtimeFailed(readErr)
	}
	if len(out) > maxRuntimeOutput {
		// a truncated listing would hide resources from cleanup and leak detection
		return nil, coded("E613", "container runtime listing exceeds the output limit")
	}
	return out, nil
}

// runtimeCommand builds the command for a binary that runtimeBinary already accepted; each branch
// names its program as a literal, so the single-exec scan sees no dynamic program.
func runtimeCommand(ctx context.Context, binary string, args []string) *osexec.Cmd {
	if binary == "podman" {
		return osexec.CommandContext(ctx, "podman", args...)
	}
	return osexec.CommandContext(ctx, "docker", args...)
}
