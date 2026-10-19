package report

// Purpose: the delivery engine of the status outbox (P7-HOST-02, Epic D7).
// It drains the durable outbox with retry (exponential backoff with jitter),
// collapse (only the highest seq per (revision, context) is posted), routing
// by remote kind (the reporter registry) and the reporting owner (a PERSONAL
// process never posts for a project whose providers.report_owner is team).
// It sits beside the CI-37 Outbox, which keeps reserving seqs and storing
// payloads; both use the same table and the same claim, so the seq rule of
// CI-37 holds: a status is never posted below the last posted seq.
// Inputs:  a store; a Registry; hooks for the remote and the owner of a status.
// Outputs: delivered statuses, entries parked as failed, suppressed or superseded.
// Constraints: a delivery failure never fails the caller; drain opportunities
// are attempt end, run end, process start, the Reconciler tick and recovery
// (Drain is cheap when nothing is due); the payload holds no secret.

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// OwnerTeam is the providers.report_owner value that makes a TEAM process the
// one that reports; OwnerPersonal is the default.
const (
	OwnerPersonal = "personal"
	OwnerTeam     = "team"
)

// SuppressedReason is stored on an entry a PERSONAL process leaves to the team.
const SuppressedReason = "reporting owner is team; this process is personal"

// Sender delivers outbox entries.
type Sender struct {
	Store    *store.Store
	Registry *Registry // nil: DefaultRegistry
	// Remote names the remote URL of the repository a status is for; it picks
	// the reporter. Nil: the GitHub remote of Owner/Repo.
	Remote func(Status) string
	// Owner returns providers.report_owner for the status's project
	// (OwnerPersonal or OwnerTeam). Nil: OwnerPersonal.
	Owner func(ctx context.Context, s Status) string
	Now   func() time.Time // nil: time.Now
	Rand  func() float64   // nil: math/rand; a value in [0,1)
}

// Default is the process-wide sender; the composition root sets its Store.
var Default = &Sender{}

// Drain delivers whatever is due through the process-wide sender.
func Drain(ctx context.Context) (int, error) { return Default.Drain(ctx) }

var errNoStore = errors.New("report: no outbox store configured")

func (d *Sender) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d *Sender) rnd() float64 {
	if d.Rand != nil {
		return d.Rand()
	}
	return rand.Float64()
}

func (d *Sender) registry() *Registry {
	if d.Registry != nil {
		return d.Registry
	}
	return DefaultRegistry
}

func (d *Sender) remote(s Status) string {
	if d.Remote != nil {
		return d.Remote(s)
	}
	return "https://github.com/" + s.Owner + "/" + s.Repo
}

// Reserve takes the seq a run will use for its final status.
func (d *Sender) Reserve(ctx context.Context, owner, repo, revision string) (int64, error) {
	if d.Store == nil {
		return 0, errNoStore
	}
	return d.Store.OutboxReserve(ctx, revision, key(owner, repo))
}

// Complete stores the status for seq and drains. Only a store error is
// returned: a failed delivery is parked for a later attempt.
func (d *Sender) Complete(ctx context.Context, seq int64, s Status) error {
	if d.Store == nil {
		return errNoStore
	}
	if err := s.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err = d.Store.OutboxReady(ctx, s.Revision, key(s.Owner, s.Repo), seq, body); err != nil {
		return err
	}
	_, err = d.Drain(ctx)
	return err
}

// Drain delivers every due entry and returns how many were posted. The store
// collapses first, so an entry a newer seq replaces is superseded without a post; entries whose reporting
// owner is another process are suppressed; failures are parked with a backoff.
func (d *Sender) Drain(ctx context.Context) (int, error) {
	if d.Store == nil {
		return 0, errNoStore
	}
	n := 0
	for round := 0; round < healRounds; round++ {
		due, err := d.Store.OutboxDue(ctx, d.now())
		if err != nil {
			return n, err
		}
		again := false
		for _, it := range due {
			posted, lost, err := d.deliver(ctx, it)
			if posted {
				n++
			}
			again = again || lost
			if err != nil {
				return n, err
			}
		}
		if !again {
			break
		}
	}
	return n, nil
}

// deliver posts one entry. lost is true when its claim was lost and the store
// queued the newest status again; the caller drains once more.
func (d *Sender) deliver(ctx context.Context, it store.OutboxItem) (posted, lost bool, err error) {
	var s Status
	if json.Unmarshal(it.Payload, &s) != nil || s.Validate() != nil {
		return false, false, nil // a corrupt entry is left for the operator; it must not block the rest
	}
	if d.owner(ctx, s) == OwnerTeam && ProcessRole() == RolePersonal {
		return false, false, d.Store.OutboxSuppress(ctx, it.Revision, it.Context, it.Seq, SuppressedReason)
	}
	res, token, err := d.Store.OutboxClaim(ctx, it.Revision, it.Context, it.Seq)
	if err != nil || res != store.ClaimOK {
		return false, false, err // superseded, or another process is posting
	}
	bg := context.WithoutCancel(ctx)
	_, rep, ok := d.registry().Resolve(d.remote(s))
	var postErr error
	if !ok {
		postErr = errors.New("no reporter accepts the remote of " + s.Owner + "/" + s.Repo)
	} else {
		postErr = rep.Post(ctx, s)
	}
	if postErr != nil {
		next := d.now().Add(Backoff(it.Attempts, d.rnd()))
		err = d.Store.OutboxBackoff(bg, it.Revision, it.Context, it.Seq, token, it.Attempts+1, next, cause(postErr))
		if store.IsClaimLost(err) {
			return false, true, nil
		}
		return false, false, err
	}
	err = d.Store.OutboxPosted(bg, it.Revision, it.Context, it.Seq, token)
	if store.IsClaimLost(err) { // suspended past the claim TTL: a newer status may have been overwritten
		return true, true, nil
	}
	return true, false, err
}

func (d *Sender) owner(ctx context.Context, s Status) string {
	if d.Owner == nil {
		return OwnerPersonal
	}
	return d.Owner(ctx, s)
}

// cause is the stored text of a failed post: one short line.
func cause(err error) string {
	m := firstLine(err.Error())
	if len(m) > 200 {
		m = m[:200]
	}
	return m
}
