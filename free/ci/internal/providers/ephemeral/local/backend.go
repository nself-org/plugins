package local

// Purpose: the seam between the provider and the P7-TRUST isolation adapters. A Backend
// answers what the host can deliver now, prepares and removes the sandbox of one attempt,
// and lists the sandboxes the runtime still holds. Everything that starts or stops a
// container or a VM stays in the adapters and in internal/exec; the provider owns the
// reservation, the labels and the accounting around them.
// Inputs:  an isolation.Attempt (id = the intent token, coordinator id).
// Outputs: Offer (level and capacity), Found (leftover sandboxes with labels).
// Constraints: a Backend never offers a level below container (no silent downgrade);
// Probe returns an E698 *Error when nothing can be offered and a nil error with a full
// Offer when the host is merely busy; Teardown is idempotent.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Offer is what a Backend can deliver right now.
type Offer struct {
	// Level is the isolation level a sandbox would have (container, sandboxed-container or vm).
	Level model.Isolation
	// Total is how many sandboxes the runtime lets run at once, ours and others'; -1 means no
	// runtime limit (Linux). Apple's two-VM limit is reported here, minus VMs that are not ours.
	Total int
	// Running names (without the nself-ci- prefix) the sandboxes the runtime holds now whose slots count against
	// Total. The provider treats a name it has no reservation for as used capacity it cannot give out.
	Running []string
}

// Found is a sandbox the runtime still holds that the provider can label completely.
type Found struct {
	ID        string
	Labels    map[string]string
	CreatedAt time.Time
}

// Backend is one platform's view of the adapters.
type Backend interface {
	// Platform is the os/arch the sandboxes run (`linux/amd64`, `darwin/arm64`).
	Platform() string
	// Probe reports the level and capacity now. An E698 *Error means no sandbox can be offered.
	Probe(ctx context.Context) (Offer, error)
	// Prepare checks, before a slot is handed out, that the adapter still works. The Tart adapter
	// also sweeps clones whose owner died here.
	Prepare(ctx context.Context, a isolation.Attempt) error
	// Teardown removes the sandbox of the attempt; it is idempotent and not-found is success.
	Teardown(ctx context.Context, a isolation.Attempt) error
	// Leftovers lists the sandboxes the runtime holds that carry the ephemeral role.
	Leftovers(ctx context.Context) ([]Found, error)
}
