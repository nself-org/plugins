package store

// Purpose: the store set host-provider (P7-HOST-16): what the hosted provider runtime remembers between
// processes. provider_fact holds the latest observed provider facts (health, quota, repo lists) by
// (provider, scope, key); provider_mark holds named sweep and poll marks (the last reconcile pass,
// the last demand scan); dispatch_intent holds the handle of a hosted dispatch so a restarted
// coordinator polls the run it already started and never dispatches the attempt a second time.
// Inputs:  provider ids, scopes, keys, JSON values (no secrets), timestamps, handles.
// Outputs: the stored rows and aggregates (prefix list, prefix count).
// Constraints: SQL lives only here; every write is one transaction; a fact never moves to an older
//          observation; the migration set is expand-only; values are opaque JSON and never carry a token.

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"time"
)

//go:embed migrations/host-provider/*.sql
var hostProviderMigrations embed.FS

func init() {
	if err := Register("host-provider", hostProviderMigrations); err != nil {
		panic(err)
	}
}

// ProviderFact is one observed provider fact.
type ProviderFact struct {
	Provider, Scope, Key string
	Value                json.RawMessage
	ObservedAt           time.Time
}

// DispatchIntent is the handle of a hosted dispatch, kept per attempt. Epoch is the fencing epoch of
// the lease the run was dispatched for: a retried attempt has a higher one.
type DispatchIntent struct {
	AttemptID, Provider, Nonce, RunID, Ref string
	Epoch                                  int64
	CreatedAt                              time.Time
}

// ProviderFactPut stores a fact. An observation older than the stored one is dropped (false).
func (s *Store) ProviderFactPut(ctx context.Context, f ProviderFact) (bool, error) {
	if f.Provider == "" || f.Key == "" || f.ObservedAt.IsZero() || !json.Valid(f.Value) {
		return false, coded("E607", "provider fact needs a provider, a key, an observation time and a JSON value")
	}
	var kept bool
	err := s.write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO provider_fact(provider,scope,key,value_json,observed_at) VALUES (?,?,?,?,?)
ON CONFLICT(provider,scope,key) DO UPDATE SET value_json=excluded.value_json, observed_at=excluded.observed_at WHERE excluded.observed_at >= provider_fact.observed_at`,
			f.Provider, f.Scope, f.Key, []byte(f.Value), f.ObservedAt.UnixNano())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		kept = n > 0
		return err
	})
	return kept, err
}

// ProviderFactGet returns one fact; ok is false when none was stored.
func (s *Store) ProviderFactGet(ctx context.Context, provider, scope, key string) (ProviderFact, bool, error) {
	var f ProviderFact
	var value []byte
	var at int64
	err := s.readers.QueryRowContext(ctx, `SELECT provider,scope,key,value_json,observed_at FROM provider_fact WHERE provider=? AND scope=? AND key=?`,
		provider, scope, key).Scan(&f.Provider, &f.Scope, &f.Key, &value, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderFact{}, false, nil
	}
	if err != nil {
		return ProviderFact{}, false, err
	}
	f.Value, f.ObservedAt = value, time.Unix(0, at).UTC()
	return f, true, nil
}

// ProviderFactList returns the facts of (provider, scope) whose key starts with prefix, ordered by key.
// The prefix is compared literally: %, _ and \ carry no wildcard meaning.
func (s *Store) ProviderFactList(ctx context.Context, provider, scope, prefix string) ([]ProviderFact, error) {
	rows, err := s.readers.QueryContext(ctx, `SELECT key,value_json,observed_at FROM provider_fact WHERE provider=? AND scope=? AND substr(key,1,length(?))=? ORDER BY key`,
		provider, scope, prefix, prefix)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ProviderFact
	for rows.Next() {
		f := ProviderFact{Provider: provider, Scope: scope}
		var value []byte
		var at int64
		if err := rows.Scan(&f.Key, &value, &at); err != nil {
			return nil, err
		}
		f.Value, f.ObservedAt = value, time.Unix(0, at).UTC()
		out = append(out, f)
	}
	return out, rows.Err()
}

// ProviderFactCount counts the facts ProviderFactList would return.
func (s *Store) ProviderFactCount(ctx context.Context, provider, scope, prefix string) (int, error) {
	var n int
	err := s.readers.QueryRowContext(ctx, `SELECT count(*) FROM provider_fact WHERE provider=? AND scope=? AND substr(key,1,length(?))=?`,
		provider, scope, prefix, prefix).Scan(&n)
	return n, err
}

// ProviderMarkPut records that name did something for key at the given time. A mark never moves back.
func (s *Store) ProviderMarkPut(ctx context.Context, name, key string, at time.Time) error {
	if name == "" || at.IsZero() {
		return coded("E607", "provider mark needs a name and a time")
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO provider_mark(name,key,at) VALUES (?,?,?)
ON CONFLICT(name,key) DO UPDATE SET at=excluded.at WHERE excluded.at >= provider_mark.at`, name, key, at.UnixNano())
		return err
	})
}

// ProviderMarkGet returns a mark; ok is false when it was never set.
func (s *Store) ProviderMarkGet(ctx context.Context, name, key string) (time.Time, bool, error) {
	var at int64
	err := s.readers.QueryRowContext(ctx, `SELECT at FROM provider_mark WHERE name=? AND key=?`, name, key).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return time.Unix(0, at).UTC(), true, nil
}

// DispatchIntentPut records the handle of an attempt's dispatch and returns the intent that stands.
// A higher Epoch (the retry of a lost attempt) replaces the stored intent; an equal or lower one
// changes nothing, so two carriers of one lease cannot repoint the attempt, and the loser sees the
// winner's run id in the result.
func (s *Store) DispatchIntentPut(ctx context.Context, d DispatchIntent) (DispatchIntent, error) {
	if d.AttemptID == "" || d.Provider == "" || d.RunID == "" {
		return DispatchIntent{}, coded("E607", "dispatch intent needs an attempt, a provider and a run")
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = s.now()
	}
	var out DispatchIntent
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO dispatch_intent(attempt_id,provider,nonce,run_id,ref,epoch,created_at) VALUES (?,?,?,?,?,?,?)
ON CONFLICT(attempt_id) DO UPDATE SET provider=excluded.provider, nonce=excluded.nonce, run_id=excluded.run_id, ref=excluded.ref, epoch=excluded.epoch, created_at=excluded.created_at
WHERE excluded.epoch > dispatch_intent.epoch`, d.AttemptID, d.Provider, d.Nonce, d.RunID, d.Ref, d.Epoch, d.CreatedAt.UnixNano()); err != nil {
			return err
		}
		var at int64
		err := tx.QueryRowContext(ctx, `SELECT attempt_id,provider,nonce,run_id,ref,epoch,created_at FROM dispatch_intent WHERE attempt_id=?`, d.AttemptID).
			Scan(&out.AttemptID, &out.Provider, &out.Nonce, &out.RunID, &out.Ref, &out.Epoch, &at)
		out.CreatedAt = time.Unix(0, at).UTC()
		return err
	})
	return out, err
}

// DispatchIntentDelete forgets the intent of an attempt when it still names runID, so a late
// cleanup of an old run cannot remove the intent of its retry.
func (s *Store) DispatchIntentDelete(ctx context.Context, attemptID, runID string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM dispatch_intent WHERE attempt_id=? AND run_id=?`, attemptID, runID)
		return err
	})
}

// DispatchIntentGet returns the stored dispatch of an attempt; ok is false when it was never dispatched.
func (s *Store) DispatchIntentGet(ctx context.Context, attemptID string) (DispatchIntent, bool, error) {
	var d DispatchIntent
	var at int64
	err := s.readers.QueryRowContext(ctx, `SELECT attempt_id,provider,nonce,run_id,ref,epoch,created_at FROM dispatch_intent WHERE attempt_id=?`, attemptID).
		Scan(&d.AttemptID, &d.Provider, &d.Nonce, &d.RunID, &d.Ref, &d.Epoch, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return DispatchIntent{}, false, nil
	}
	if err != nil {
		return DispatchIntent{}, false, err
	}
	d.CreatedAt = time.Unix(0, at).UTC()
	return d, true, nil
}
