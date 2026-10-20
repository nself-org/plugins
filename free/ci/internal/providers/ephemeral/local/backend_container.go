package local

// Purpose: the Linux backend: gVisor sandboxed-container where runsc really starts, the
// hardened container profile where it does not, and E698 when there is no runtime at all.
// Inputs:  the registered `sandboxed-container` and `container` profiles (P7-TRUST-04/05).
// Outputs: an Offer, a Teardown through the profile, leftovers read from the runtime by label.
// Constraints: the runtime CLI is reached only through isolation.Profile and exec.RuntimeQuery
// (the exec funnel); containers are named nself-ci-<attempt> and carry role, coordinator_id and
// nself.ci.attempt labels set by the container profile; the attempt id is the intent token, so a
// leftover container maps back to its intent.

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

const (
	containerLevel = "container"
	sandboxLevel   = "sandboxed-container"
	// namePrefix and the label keys are the container profile's (isolation/container/flags.go).
	namePrefix   = "nself-ci-"
	attemptLabel = "nself.ci.attempt"
	cleanSuffix  = "-clean"
)

// Query asks the container runtime a read-only question (exec.RuntimeQuery in production).
type Query func(ctx context.Context, binary string, args ...string) ([]byte, error)

type containerBackend struct {
	container isolation.Profile
	sandbox   isolation.Profile // nil when no gVisor profile is registered
	query     Query
	platform  string
}

// NewContainerBackend builds the Linux backend over the two profiles; sandbox may be nil.
func NewContainerBackend(container, sandbox isolation.Profile, q Query) Backend {
	if q == nil {
		q = exec.RuntimeQuery
	}
	return &containerBackend{container: container, sandbox: sandbox, query: q, platform: "linux/" + runtime.GOARCH}
}

func (b *containerBackend) Platform() string { return b.platform }

func (b *containerBackend) Probe(ctx context.Context) (Offer, error) {
	if b.container == nil {
		return Offer{}, unavailable(ReasonNoAdapter, nil, "the container isolation profile is not registered")
	}
	av := b.container.Probe(ctx)
	if !av.Available() {
		return Offer{}, unavailable(ReasonNoRuntime, nil, "%s", reasonText(av))
	}
	level := isolation.Effective(b.container.Level(), av.Level)
	if b.sandbox != nil {
		if sav := b.sandbox.Probe(ctx); sav.Available() {
			level = isolation.Effective(b.sandbox.Level(), sav.Level)
		}
	}
	return Offer{Level: level, Total: -1}, nil
}

func (b *containerBackend) Prepare(ctx context.Context, a isolation.Attempt) error {
	return b.container.Prepare(ctx, a)
}

func (b *containerBackend) Teardown(ctx context.Context, a isolation.Attempt) error {
	return b.container.Teardown(ctx, a)
}

// Leftovers lists containers carrying role=ci-ephemeral, running or not, with the labels
// and creation time the runtime reports. A runtime that cannot be read is an error, never an empty list.
func (b *containerBackend) Leftovers(ctx context.Context) ([]Found, error) {
	lister, ok := b.container.(isolation.Lister)
	if !ok {
		return nil, unavailable(ReasonNoAdapter, nil, "the container profile cannot list its containers")
	}
	names, err := lister.List(ctx, "role="+roleValue)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	av := b.container.Probe(ctx)
	bin := av.Facts["runtime"]
	if bin == "" {
		return nil, unavailable(ReasonNoRuntime, nil, "the container profile reported no runtime binary")
	}
	args := append([]string{"inspect", "--format", `{{.Name}}|{{.Created}}|{{json .Config.Labels}}`}, names...)
	out, qerr := b.query(ctx, bin, args...)
	found := parseInspect(string(out))
	if qerr != nil && len(found) == 0 {
		return nil, qerr // nothing readable: not "no leftovers"
	}
	return found, nil // a container that vanished between ps and inspect only shortens the output
}

// parseInspect turns `Name|Created|{labels}` lines into Found values; a line it cannot read is skipped.
func parseInspect(out string) []Found {
	var res []Found
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 {
			continue
		}
		name := strings.TrimPrefix(parts[0], "/")
		created, err := time.Parse(time.RFC3339Nano, parts[1])
		var labels map[string]string
		id := strings.TrimPrefix(name, namePrefix)
		// only ids in this provider's namespace: ordinary job containers carry the same labels and are not ours to list
		if err != nil || json.Unmarshal([]byte(parts[2]), &labels) != nil || !strings.HasPrefix(name, namePrefix) || labels[roleKey] != roleValue || !idRe.MatchString(id) {
			continue
		}
		f := Found{ID: id, CreatedAt: created, Labels: map[string]string{roleKey: roleValue, coordKey: labels[coordKey]}}
		// only the job container itself (nself-ci-<attempt>) stands for its intent; the cleanup helper never does
		if attempt := labels[attemptLabel]; attempt != "" && id == attempt && !strings.HasSuffix(id, cleanSuffix) {
			f.Labels[intentKey] = strings.TrimPrefix(attempt, idPrefix)
		}
		res = append(res, f)
	}
	return res
}
