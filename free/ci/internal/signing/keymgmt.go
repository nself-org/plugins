package signing

// Purpose: create and replace the release key (Epic CACHE D11): `key init` and
// `key rotate` run these inside the signer process, so the generated key is
// never seen by the coordinator or the operator's shell.
// Inputs:  a secrets.ValueResolver, a SecretStore and an attempt label.
// Outputs: the KeyInfo of the new key (and the old id for a rotation).
// Constraints: init refuses when the secret exists (secret_exists) and
// stores nothing; rotate needs a working key (so `key_missing` names init) and
// updates a stored `..._KEY_ID` too, otherwise the next load would fail
// key_id_mismatch; the value reaches `config secrets set --env release` only
// on stdin; every key and value buffer is zeroed.

import (
	"context"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/secrets"
)

// KeyIDName is the secret that may hold the key id beside the key.
const KeyIDName = "NSELF_CI_RELEASE_SIGNING_KEY_ID"

// KeyAttempt labels key-management calls to the resolver (no run is involved).
const KeyAttempt = "ci-artifacts-key"

// SecretStore writes a release-class secret.
type SecretStore interface {
	// Probe reports cli_too_old when no `config secrets set` spelling exists.
	Probe(ctx context.Context) error
	// Set stores value under name in the release store file; value is on stdin only.
	Set(ctx context.Context, name string, value []byte) error
	// Lock serializes key management across processes; call the result to release.
	Lock(ctx context.Context) (func(), error)
}

// Init generates the key and stores it. It refuses when a key is already set.
func Init(ctx context.Context, res secrets.ValueResolver, st SecretStore) (KeyInfo, error) {
	if err := st.Probe(ctx); err != nil {
		return KeyInfo{}, err
	}
	unlock, err := st.Lock(ctx)
	if err != nil {
		return KeyInfo{}, err
	}
	defer unlock()
	old, err := loadKey(ctx, res, KeyAttempt, false)
	if err == nil {
		old.Zero()
		return KeyInfo{}, Failf(ReasonSecretExists, "the release key is already set: use `key rotate` to replace it")
	}
	if !IsReason(err, ReasonKeyMissing) {
		return KeyInfo{}, err
	}
	return storeNewKey(ctx, res, st, "")
}

// Rotate replaces the stored key and returns the new trust line with the old id.
func Rotate(ctx context.Context, res secrets.ValueResolver, st SecretStore) (KeyInfo, error) {
	if err := st.Probe(ctx); err != nil {
		return KeyInfo{}, err
	}
	unlock, err := st.Lock(ctx)
	if err != nil {
		return KeyInfo{}, err
	}
	defer unlock()
	old, err := loadKey(ctx, res, KeyAttempt, false) // a stale stored id is what a rotation repairs
	if err != nil {
		return KeyInfo{}, err
	}
	oldID := old.ID
	old.Zero()
	return storeNewKey(ctx, res, st, oldID)
}

// storeNewKey generates a key, stores it and, when an id is stored beside the
// key, updates that too (otherwise the next load fails key_id_mismatch).
func storeNewKey(ctx context.Context, res secrets.ValueResolver, st SecretStore, oldID string) (KeyInfo, error) {
	hadID := true
	if raw, err := resolveOne(ctx, res, KeyAttempt, KeyIDRef); IsReason(err, ReasonKeyMissing) {
		hadID = false
	} else if err != nil {
		return KeyInfo{}, err
	} else {
		clear(raw)
	}
	key, value, err := Generate()
	if err != nil {
		return KeyInfo{}, err
	}
	defer key.Zero()
	defer clear(value)
	if err := st.Set(ctx, KeyName, value); err != nil {
		return KeyInfo{}, err
	}
	if hadID {
		if err := st.Set(ctx, KeyIDName, []byte(key.ID)); err != nil {
			return KeyInfo{}, Failf(ReasonKeyIDMismatch, "the new key %s was stored but %s could not be updated: run `nself ci artifacts key rotate` again, or set it to %s", key.ID, KeyIDName, key.ID)
		}
	}
	return KeyInfo{KeyID: key.ID, Public: key.PublicB64(), OldKeyID: oldID}, nil
}

// CLIStore stores secrets with the nself cli through a StdioRunner.
type CLIStore struct {
	Runner StdioRunner
	// Binary is the cli program; "" means "nself".
	Binary string
	// Project is the project root the cli runs in (the store is found from it).
	Project string
	Env     []string
	// LockDir holds the per-project lock file that serializes key management
	// ("" = no lock).
	LockDir string

	spelling []string
}

func (s *CLIStore) binary() string {
	if s.Binary == "" {
		return "nself"
	}
	return s.Binary
}

// Probe asks `config secrets set --help`, then the v1.4 spelling `secrets set --help`.
func (s *CLIStore) Probe(ctx context.Context) error {
	if s.spelling != nil {
		return nil
	}
	for _, sp := range [][]string{{"config", "secrets", "set"}, {"secrets", "set"}} {
		argv := append(append([]string{s.binary()}, sp...), "--help")
		out, code, err := s.Runner.RunStdio(ctx, StdioCall{Argv: argv, Env: s.Env, Dir: s.Project, Timeout: secrets.CallTimeout})
		clear(out)
		if err == nil && code == 0 {
			s.spelling = sp
			return nil
		}
	}
	return Failf(ReasonCLITooOld, "this nself cli has neither `config secrets set` nor `secrets set`: upgrade it")
}

// Set runs `nself <spelling> --env release NAME` with value on stdin.
func (s *CLIStore) Set(ctx context.Context, name string, value []byte) error {
	if err := s.Probe(ctx); err != nil {
		return err
	}
	argv := append(append([]string{s.binary()}, s.spelling...), "--env", "release", name)
	out, code, err := s.Runner.RunStdio(ctx, StdioCall{Argv: argv, Env: s.Env, Dir: s.Project, Stdin: value, Timeout: 30 * time.Second})
	clear(out)
	if err != nil || code != 0 {
		return Failf(ReasonSignerFailed, "`%s` failed (exit %d)", strings.Join(s.spelling, " "), code)
	}
	return nil
}
