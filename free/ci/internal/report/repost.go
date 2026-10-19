package report

// Purpose: replace a status that is no longer true (P7-HOST-02, Epic D7). When
// a node's key is revoked, the success it earned must not stand: Repost
// enqueues a new status with a NEW seq for each affected (revision, context),
// so the replacement is posted after the old success and a completion that
// arrives later with an older seq cannot overwrite it (the seq rule of CI-37).
// P7-TRUST-20 calls it with the revocation's re-evaluation set.
// Inputs:  keys, the state to post (usually Error) and the reason.
// Outputs: queued entries, delivered by the sender's normal drain.
// Constraints: the description is the reason cut to 140 runes; a key the
// outbox cannot parse or a state that is not valid is refused whole.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Key names one outbox stream: the commit and the repository's status context.
type Key struct {
	Revision string
	Context  string // the store key, "nself-ci@owner/repo"
}

// KeyFor is the Key of the status of owner/repo at revision.
func KeyFor(owner, repo, revision string) Key {
	return Key{Revision: revision, Context: key(owner, repo)}
}

func (k Key) ownerRepo() (owner, repo string, ok bool) {
	rest, found := strings.CutPrefix(k.Context, Context+"@")
	if !found {
		return "", "", false
	}
	owner, repo, ok = strings.Cut(rest, "/")
	return owner, repo, ok
}

// Repost enqueues state with reason as a new seq for every key and drains. It
// returns every key that could not be queued, joined; the others still post.
func (d *Sender) Repost(ctx context.Context, keys []Key, state State, reason string) error {
	if d.Store == nil {
		return errNoStore
	}
	var errs []error
	for _, k := range keys {
		owner, repo, ok := k.ownerRepo()
		if !ok {
			errs = append(errs, fmt.Errorf("report: repost: not a status key: %q", k.Context))
			continue
		}
		s := Status{Owner: owner, Repo: repo, Revision: k.Revision, State: state, Description: Truncate(reason)}
		body, err := json.Marshal(s)
		if err == nil {
			err = s.Validate()
		}
		if err == nil {
			_, err = d.Store.OutboxEnqueue(ctx, k.Revision, k.Context, body)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("report: repost %s: %w", k.Revision, err))
		}
	}
	_, err := d.Drain(ctx)
	return errors.Join(append(errs, err)...)
}

// Repost queues the replacement through the process-wide sender.
func Repost(ctx context.Context, keys []Key, state State, reason string) error {
	return Default.Repost(ctx, keys, state, reason)
}
