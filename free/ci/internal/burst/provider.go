package burst

// Purpose: the ephemeral provider contract (Epic D13), owned here and not by
// P7-HOST: a provider can create a one-job machine, delete it and list what it
// holds by label. Everything the lifecycle guarantees (no leak, no duplicate,
// no foreign deletion) rests on these five calls and on the labels.
// Inputs:  an Intent (what to create, labelled) and, for List and Teardown, labels and ids.
// Outputs: Resource values the lifecycle records and the sweeper reasons over.
// Constraints: Provision is idempotent by the `intent` label (a second call for
// the same intent returns the existing resource); Teardown is idempotent and
// treats not-found as success; List returns only resources carrying every
// requested label; every resource carries role, coordinator_id and intent, each
// value at most 63 characters; CreatedAt is the provider's own clock reading and
// never zero; no method may block past its context.

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

// The labels every ephemeral resource carries (Epic D12).
const (
	LabelRole        = "role"
	LabelCoordinator = "coordinator_id"
	LabelIntent      = "intent"
	// RoleEphemeral is the role value; the sweeper touches nothing else.
	RoleEphemeral = "ci-ephemeral"
	// MaxLabelValue is the longest label value a provider is guaranteed to accept.
	MaxLabelValue = 63
)

// Class values of Provider.Class.
const (
	ClassCloud = "cloud"
	ClassLocal = "local"
)

// Spec is what a job needs from a new machine.
type Spec struct {
	Platform string
	CPU      int
	MemMB    int
	// TimeoutMs is the job timeout; the provider prices the machine for its lifetime from it.
	TimeoutMs int64
}

// Estimate is a provider's own figure for a Spec. LatencyMs 0 means unknown.
type Estimate struct {
	LatencyMs int64
	// Class is the economics cost class (`ephemeral` for a cloud VM, `local` otherwise).
	Class string
}

// Intent asks a provider for one machine. Labels always include the three
// lifecycle labels; Token is the same value as Labels[LabelIntent].
type Intent struct {
	Token  string
	Spec   Spec
	Labels map[string]string
}

// Resource is one machine at a provider.
type Resource struct {
	ID        string
	Name      string
	Labels    map[string]string
	Addrs     []netip.Addr
	CreatedAt time.Time
	// NodeID is the node-registry id once the machine joined; empty before.
	NodeID string
}

// Provider is the ephemeral-provider interface (Epic D13).
type Provider interface {
	ID() string
	Class() string
	Estimate(ctx context.Context, s Spec) (Estimate, error)
	Provision(ctx context.Context, in Intent) (Resource, error)
	Teardown(ctx context.Context, id string) error
	List(ctx context.Context, labels map[string]string) ([]Resource, error)
}

// Errors a provider returns; the lifecycle classifies by errors.Is.
var (
	// ErrNotFound: the resource does not exist (Teardown maps it to success).
	ErrNotFound = errors.New("burst: resource not found")
	// ErrUnavailable: the provider API cannot be reached; retry later.
	ErrUnavailable = errors.New("burst: provider unavailable")
	// ErrQuota: the provider refused for quota or capacity; do not retry now.
	ErrQuota = errors.New("burst: provider quota")
)

// IntentLabels returns the lifecycle labels of an intent.
func IntentLabels(coordinator, token string) map[string]string {
	return map[string]string{LabelRole: RoleEphemeral, LabelCoordinator: coordinator, LabelIntent: token}
}

// OwnLabels returns the selector that finds every resource of one coordinator.
func OwnLabels(coordinator string) map[string]string {
	return map[string]string{LabelRole: RoleEphemeral, LabelCoordinator: coordinator}
}

// Mine reports whether labels carry the ephemeral role and this coordinator's id.
func Mine(labels map[string]string, coordinator string) bool {
	return coordinator != "" && labels[LabelRole] == RoleEphemeral && labels[LabelCoordinator] == coordinator
}

// Matches reports whether labels carry every key and value of want.
func Matches(labels, want map[string]string) bool {
	for k, v := range want {
		if got, ok := labels[k]; !ok || got != v {
			return false
		}
	}
	return true
}
