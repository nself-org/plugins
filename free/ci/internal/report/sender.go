package report

// Purpose: the delivery engine of the status outbox (P7-HOST-02, Epic D7).
// It drains the durable outbox with retry (exponential backoff with jitter),
// collapse (only the highest seq per (revision, context) is posted), routing
// by remote kind (the reporter registry) and the reporting owner (a PERSONAL
// process never posts for a project whose providers.report_owner is team).
// It sits beside the CI-37 Outbox, which keeps reserving seqs and storing
// payloads; both use the same table and the same claim (store.OutboxClaim,
// which also counts a parked newer seq), so the seq rule of CI-37 holds in
// either engine: a status is never posted below a newer seq that was posted,
// is being posted or is parked for a retry.
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
	"strings"
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
	// the reporter. Nil: the GitHub remote of Owner/Repo, so a composition root
	// that registers a second kind MUST set it, or that kind never receives a post.
	Remote func(Status) string
	// Owner returns providers.report_owner for the status's project
	// (OwnerPersonal or OwnerTeam; providers.ReportOwner reads it from the
	// effective policy). Nil: OwnerPersonal, so a composition root that wants
	// team ownership honoured MUST set it, or nothing is ever suppressed.
	Owner func(ctx context.Context, s Status) string
	Now   func() time.Time // nil: time.Now
	Rand  func() float64   // nil: math/rand; a value in [0,1)
	Pause time.Duration    // wait between claim retries while an older post is in flight; zero: 250 ms
}

// Default is the process-wide sender; the composition root sets its Store,
// Remote and Owner (see the field notes: unset hooks fail open).
var Default = &Sender{}

// postTimeout bounds one Post; it stays below the store's claim TTL (60 s), so
// a hung reporter cannot make the delivery lose its claim.
const postTimeout = 30 * time.Second

// claimWaits bounds how often deliver retries while an older seq is mid-post.
const claimWaits = 10

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
	if err = d.Store.OutboxReady(ctx, s.Revision, keyFor(s), seq, body); err != nil {
		return err
	}
	_, err = d.Drain(ctx)
	return err
}

// Drain delivers every due entry and returns how many were posted. The store
// collapses first, so an entry a newer seq replaces is superseded without a
// post; entries whose reporting owner is another process are suppressed (and
// revived when this process becomes the owner); failures are parked with a backoff.
func (d *Sender) Drain(ctx context.Context) (int, error) {
	if d.Store == nil {
		return 0, errNoStore
	}
	n := 0
	for round := 0; round < healRounds; round++ {
		if err := d.revive(ctx); err != nil {
			return n, err
		}
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

// suppressed reports whether this process must leave s to another reporter.
func (d *Sender) suppressed(ctx context.Context, s Status) bool {
	return d.owner(ctx, s) == OwnerTeam && ProcessRole() == RolePersonal
}

// revive requeues the suppressed entries this process may deliver now: a TEAM
// process started on the same store, or the owner key went back to personal.
func (d *Sender) revive(ctx context.Context) error {
	list, err := d.Store.OutboxList(ctx)
	if err != nil {
		return err
	}
	for _, it := range list {
		var s Status
		if it.State != store.OutboxSuppressed || it.Reason != SuppressedReason || json.Unmarshal(it.Payload, &s) != nil || s.Validate() != nil || d.suppressed(ctx, s) {
			continue
		}
		if err = d.Store.OutboxRequeue(ctx, it.Revision, it.Context, it.Seq); err != nil {
			return err
		}
	}
	return nil
}

// claim takes the right to post seq, waiting a little while an older seq is
// mid-post (the CI-37 Outbox waits longer). ok is false when the entry must not
// be posted now: superseded, another process is posting it, or the wait ran out.
func (d *Sender) claim(ctx context.Context, it store.OutboxItem) (token int64, ok bool, err error) {
	for i := 0; ; i++ {
		var res int
		if res, token, err = d.Store.OutboxClaim(ctx, it.Revision, it.Context, it.Seq); err != nil || res == store.ClaimOK {
			return token, err == nil, err
		}
		if res != store.ClaimWait || i >= claimWaits {
			return 0, false, nil
		}
		select {
		case <-ctx.Done():
			return 0, false, ctx.Err()
		case <-time.After(d.pause()):
		}
	}
}

func (d *Sender) pause() time.Duration {
	if d.Pause > 0 {
		return d.Pause
	}
	return 250 * time.Millisecond
}

// deliver posts one entry. lost is true when the store queued another status
// for delivery (a lost claim healed, or a status revived); the caller drains once more.
func (d *Sender) deliver(ctx context.Context, it store.OutboxItem) (posted, lost bool, err error) {
	var s Status
	if json.Unmarshal(it.Payload, &s) != nil || s.Validate() != nil {
		// Parked where `ci runs outbox` shows it; the readable status it superseded
		// is queued again, because this entry can never be posted.
		if err = d.Store.OutboxSuppress(ctx, it.Revision, it.Context, it.Seq, "unreadable payload"); err != nil {
			return false, false, err
		}
		return false, true, d.Store.OutboxReviveBelow(ctx, it.Revision, it.Context, it.Seq) // lost: drain again
	}
	if d.suppressed(ctx, s) {
		return false, false, d.Store.OutboxSuppress(ctx, it.Revision, it.Context, it.Seq, SuppressedReason)
	}
	token, ok, err := d.claim(ctx, it)
	if err != nil || !ok {
		return false, false, err
	}
	bg := context.WithoutCancel(ctx)
	_, rep, found := d.registry().Resolve(d.remote(s))
	var postErr error
	if !found {
		postErr = errors.New("no reporter accepts the remote of " + s.Owner + "/" + s.Repo)
	} else {
		pctx, cancel := context.WithTimeout(ctx, postTimeout)
		postErr = rep.Post(pctx, s)
		cancel()
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

// cause is the stored text of a failed post: one short line of at most 200
// runes, with control characters (terminal escapes) turned into spaces.
func cause(err error) string {
	m, _, _ := strings.Cut(err.Error(), "\n")
	r := []rune(m)
	if len(r) > 200 {
		r = r[:200]
	}
	for i, c := range r {
		if c < 0x20 || c == 0x7f {
			r[i] = ' '
		}
	}
	return string(r)
}
