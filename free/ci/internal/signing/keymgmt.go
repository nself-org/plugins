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

// KeyAttempt labels key-management calls to the resolver (no run is involved).
const KeyAttempt = "ci-artifacts-key"

// SecretStore writes a release-class secret.
type SecretStore interface {
	// Probe reports cli_too_old when no `config secrets set` spelling exists.
	Probe(ctx context.Context) error
	// Set stores value under name in the release store file; value is on stdin only.
	Set(ctx context.Context, name string, value []byte) error
}

// Init generates the key and stores it. It refuses when a key is already set.
func Init(ctx context.Context, res secrets.ValueResolver, st SecretStore) (KeyInfo, error) {
	if err := st.Probe(ctx); err != nil {
		return KeyInfo{}, err
	}
	old, err := LoadKey(ctx, res, KeyAttempt)
	if err == nil {
		old.Zero()
		return KeyInfo{}, Failf(ReasonSecretExists, "the release key is already set: use `key rotate` to replace it")
	}
	if !IsReason(err, ReasonKeyMissing) {
		return KeyInfo{}, err
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
	return KeyInfo{KeyID: key.ID, Public: key.PublicB64()}, nil
}

// Rotate replaces the stored key and returns the new trust line with the old id.
func Rotate(ctx context.Context, res secrets.ValueResolver, st SecretStore) (KeyInfo, error) {
	if err := st.Probe(ctx); err != nil {
		return KeyInfo{}, err
	}
	old, err := LoadKey(ctx, res, KeyAttempt)
	if err != nil {
		return KeyInfo{}, err
	}
	oldID := old.ID
	old.Zero()
	hadID := true
	if raw, ierr := resolveOne(ctx, res, KeyAttempt, KeyIDRef); IsReason(ierr, ReasonKeyMissing) {
		hadID = false
	} else if ierr == nil {
		clear(raw)
	} else {
		return KeyInfo{}, ierr
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
		if err := st.Set(ctx, KeyName+"_ID", []byte(key.ID)); err != nil {
			return KeyInfo{}, Failf(ReasonKeyIDMismatch, "the key was replaced but its stored id could not be updated: run `nself config secrets set --env release %s_ID` with %s", KeyName, key.ID)
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
