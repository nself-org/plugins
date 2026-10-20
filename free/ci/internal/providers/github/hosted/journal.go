package hosted

// Purpose: the dispatch journal (Epic D5, review F10): the intent to dispatch an attempt is written
// BEFORE the request, so a crash, a timeout or a retry never creates a second run for one attempt.
// Inputs:  a Record (attempt, attempt number, revision, spec digest, nonce, dispatch ref).
// Outputs: the record that stands for an (attempt, number), the record of an earlier number of the
//          same attempt (whose run a retry cancels), and a record looked up by nonce (Collect).
// Constraints: a nonce is minted by the provider and is never reused for another attempt number; the
//          same (attempt, number) always answers with the first nonce; a lower number than the one
//          recorded is refused (superseded). The memory journal serves tests and a coordinator with
//          no store; the store journal keeps two dispatch_intent rows per attempt number, keyed
//          "intent:<nonce>" (the record, so Collect finds what was dispatched) and
//          "latest:<attempt>" (the newest nonce, fenced by the attempt number). A record holds ids
//          and digests, never a token or the spec.

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Record is one intended dispatch.
type Record struct {
	Attempt    string `json:"attempt"`
	N          int    `json:"n"`
	Revision   string `json:"revision"`
	SpecDigest string `json:"spec_digest"`
	Nonce      string `json:"nonce"`
	Ref        string `json:"ref"`
}

// errSuperseded means a higher attempt number was already begun.
var errSuperseded = errors.New("attempt number superseded")

// Journal remembers what was dispatched.
type Journal interface {
	// Begin records r. When (r.Attempt, r.N) was begun before it returns that earlier record as stand;
	// previous is the record of a lower number of the same attempt, when there is one.
	Begin(ctx context.Context, r Record) (stand Record, previous *Record, err error)
	// ByNonce returns the record of a nonce.
	ByNonce(ctx context.Context, nonce string) (Record, bool, error)
}

type memJournal struct {
	mu     sync.Mutex
	latest map[string]Record
	nonces map[string]Record
}

// NewMemJournal returns a process-local journal.
func NewMemJournal() Journal {
	return &memJournal{latest: map[string]Record{}, nonces: map[string]Record{}}
}

func (j *memJournal) Begin(_ context.Context, r Record) (Record, *Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	old, ok := j.latest[r.Attempt]
	switch {
	case ok && old.N == r.N:
		return old, nil, nil
	case ok && old.N > r.N:
		return Record{}, nil, errSuperseded
	}
	j.latest[r.Attempt], j.nonces[r.Nonce] = r, r
	if ok {
		return r, &old, nil
	}
	return r, nil, nil
}

func (j *memJournal) ByNonce(_ context.Context, nonce string) (Record, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.nonces[nonce]
	return r, ok, nil
}

type storeJournal struct{ st *store.Store }

// NewStoreJournal returns a journal kept in the coordinator's dispatch_intent table.
func NewStoreJournal(st *store.Store) Journal { return storeJournal{st} }

const pending = "pending"

func (j storeJournal) put(ctx context.Context, key string, r Record) (store.DispatchIntent, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return store.DispatchIntent{}, err
	}
	return j.st.DispatchIntentPut(ctx, store.DispatchIntent{AttemptID: key, Provider: ProviderID, Nonce: r.Nonce, RunID: pending, Ref: string(raw), Epoch: int64(r.N)})
}

func recordOf(d store.DispatchIntent) (Record, error) {
	var r Record
	err := json.Unmarshal([]byte(d.Ref), &r)
	return r, err
}

func (j storeJournal) ByNonce(ctx context.Context, nonce string) (Record, bool, error) {
	d, ok, err := j.st.DispatchIntentGet(ctx, "intent:"+nonce)
	if err != nil || !ok {
		return Record{}, false, err
	}
	r, err := recordOf(d)
	return r, err == nil, err
}

func (j storeJournal) Begin(ctx context.Context, r Record) (Record, *Record, error) {
	var previous *Record
	if d, ok, err := j.st.DispatchIntentGet(ctx, "latest:"+r.Attempt); err != nil {
		return Record{}, nil, err
	} else if ok {
		old, found, err := j.ByNonce(ctx, d.Nonce)
		switch {
		case err != nil:
			return Record{}, nil, err
		case d.Epoch > int64(r.N):
			return Record{}, nil, errSuperseded
		case d.Epoch == int64(r.N) && found:
			return old, nil, nil
		case found:
			previous = &old
		}
	}
	if _, err := j.put(ctx, "intent:"+r.Nonce, r); err != nil {
		return Record{}, nil, err
	}
	won, err := j.put(ctx, "latest:"+r.Attempt, r)
	if err != nil {
		return Record{}, nil, err
	}
	if won.Epoch > int64(r.N) {
		return Record{}, nil, errSuperseded
	}
	if won.Nonce != r.Nonce { // another coordinator began the same number first: follow it
		stand, found, err := j.ByNonce(ctx, won.Nonce)
		if err != nil || !found {
			return Record{}, nil, errors.Join(errSuperseded, err)
		}
		return stand, previous, nil
	}
	return r, previous, nil
}
