package signing_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func TestInitStoresAKeyOnceAndRefusesASecond(t *testing.T) {
	res := &fakeRes{vals: map[string][]byte{}}
	st := &fakeStore{res: res}
	info, err := signing.Init(context.Background(), res, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.sets) != 1 || st.sets[0] != signing.KeyName || !strings.HasPrefix(info.KeyID, "ci-release-") || info.OldKeyID != "" {
		t.Fatalf("%v %+v", st.sets, info)
	}
	got, err := signing.Public(context.Background(), res, "x")
	if err != nil || got.KeyID != info.KeyID || got.Public != info.Public {
		t.Fatalf("the stored key is not the reported one: %+v %v", got, err)
	}
	if _, err := signing.Init(context.Background(), res, st); !signing.IsReason(err, signing.ReasonSecretExists) || len(st.sets) != 1 {
		t.Fatalf("second init: %v (%d sets)", err, len(st.sets))
	}
}

func TestInitFailures(t *testing.T) {
	res := &fakeRes{vals: map[string][]byte{}}
	if _, err := signing.Init(context.Background(), res, &fakeStore{res: res, probeErr: signing.Failf(signing.ReasonCLITooOld, "old")}); !signing.IsReason(err, signing.ReasonCLITooOld) || res.calls != 0 {
		t.Fatalf("probe failure: %v (%d resolves)", err, res.calls)
	}
	if _, err := signing.Init(context.Background(), res, &fakeStore{res: res, setErr: errors.New("x")}); err == nil || len(res.vals) != 0 {
		t.Fatalf("a failed set: %v", err)
	}
	// a stored value that does not parse is not "missing": init does not overwrite it
	res.vals[signing.KeyRef] = []byte("!!")
	st := &fakeStore{res: res}
	if _, err := signing.Init(context.Background(), res, st); !signing.IsReason(err, signing.ReasonKeyInvalid) || len(st.sets) != 0 {
		t.Fatalf("init over an invalid key: %v", err)
	}
}

func TestRotateReplacesTheKeyAndReportsTheOldID(t *testing.T) {
	old, value, _ := signing.Generate()
	res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
	st := &fakeStore{res: res}
	info, err := signing.Rotate(context.Background(), res, st)
	if err != nil {
		t.Fatal(err)
	}
	if info.OldKeyID != old.ID || info.KeyID == old.ID || len(st.sets) != 1 {
		t.Fatalf("%+v %v", info, st.sets)
	}
	if now, err := signing.Public(context.Background(), res, "x"); err != nil || now.KeyID != info.KeyID {
		t.Fatalf("the stored key is not the new one: %+v %v", now, err)
	}
}

func TestRotateUpdatesAStoredKeyID(t *testing.T) {
	old, value, _ := signing.Generate()
	res := &fakeRes{vals: map[string][]byte{signing.KeyRef: value, signing.KeyIDRef: []byte(old.ID)}}
	st := &fakeStore{res: res}
	info, err := signing.Rotate(context.Background(), res, st)
	if err != nil || len(st.sets) != 2 || st.sets[1] != signing.KeyName+"_ID" || string(res.vals[signing.KeyIDRef]) != info.KeyID {
		t.Fatalf("%+v %v %v", info, st.sets, err)
	}
	if _, err := signing.Public(context.Background(), res, "x"); err != nil {
		t.Fatalf("the key does not load after a rotation: %v", err)
	}
}

func TestRotateFailures(t *testing.T) {
	res := &fakeRes{vals: map[string][]byte{}}
	if _, err := signing.Rotate(context.Background(), res, &fakeStore{res: res}); !signing.IsReason(err, signing.ReasonKeyMissing) {
		t.Fatalf("rotate without a key: %v", err)
	}
	if _, err := signing.Rotate(context.Background(), res, &fakeStore{res: res, probeErr: signing.Failf(signing.ReasonCLITooOld, "old")}); !signing.IsReason(err, signing.ReasonCLITooOld) {
		t.Fatal(err)
	}
	_, value, _ := signing.Generate()
	res.vals[signing.KeyRef] = value
	if _, err := signing.Rotate(context.Background(), res, &fakeStore{res: res, setErr: errors.New("x")}); err == nil {
		t.Fatal("a failed set was ignored")
	}
}

// TestRotationRevokesTheOldKey: after a rotation, a release signed with the old
// key verifies no more once its id is in the revoked list.
func TestRotationRevokesTheOldKey(t *testing.T) {
	f := newFixture(t)
	req := request(item(signing.PayloadStatement, f.statementPayload(nil)))
	before, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
	if err != nil {
		t.Fatal(err)
	}
	st := &fakeStore{res: f.res}
	info, err := signing.Rotate(context.Background(), f.res, st)
	if err != nil {
		t.Fatal(err)
	}
	pub := func(id string, key []byte) sdk.Key {
		return sdk.Key{ID: id, Purpose: sdk.PurposeCIRelease, Public: key}
	}
	oldKey, newKey := pub(f.key.ID, f.key.Public), pub(info.KeyID, mustPub(t, info))
	trusted := []sdk.Key{oldKey, newKey}
	v, _ := sdk.NewVerifier(sdk.PurposeCIRelease, trusted, nil)
	if _, _, _, err := sdk.VerifyEnvelope(v, before[0]); err != nil {
		t.Fatalf("the old release stopped verifying before revocation: %v", err)
	}
	revoked, _ := sdk.NewVerifier(sdk.PurposeCIRelease, trusted, []string{info.OldKeyID})
	if _, _, _, err := sdk.VerifyEnvelope(revoked, before[0]); !errors.Is(err, sdk.ErrRevoked) {
		t.Fatalf("a release signed with the revoked key verified: %v", err)
	}
	after, err := f.handler().Handle(context.Background(), req, decodeAll(t, req))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ids, err := sdk.VerifyEnvelope(revoked, after[0]); err != nil || ids[0] != info.KeyID {
		t.Fatalf("a release signed with the new key: %v %v", err, ids)
	}
}

// scriptRunner answers StdioCalls from a script and records them.
type scriptRunner struct {
	calls []signing.StdioCall
	exit  func(c signing.StdioCall) (int, error)
}

func (s *scriptRunner) RunStdio(_ context.Context, c signing.StdioCall) ([]byte, int, error) {
	s.calls = append(s.calls, c)
	code, err := s.exit(c)
	return []byte("out"), code, err
}

func TestCLIStoreSpellingProbeAndStdin(t *testing.T) {
	has := func(c signing.StdioCall, word string) bool { return strings.Contains(strings.Join(c.Argv, " "), word) }
	// current spelling
	r := &scriptRunner{exit: func(signing.StdioCall) (int, error) { return 0, nil }}
	st := &signing.CLIStore{Runner: r, Project: "/proj", Env: []string{"A=b"}}
	if err := st.Set(context.Background(), signing.KeyName, []byte("VALUE")); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 2 || strings.Join(r.calls[0].Argv, " ") != "nself config secrets set --help" {
		t.Fatalf("%v", r.calls)
	}
	set := r.calls[1]
	if strings.Join(set.Argv, " ") != "nself config secrets set --env release "+signing.KeyName || string(set.Stdin) != "VALUE" || set.Dir != "/proj" || set.Env[0] != "A=b" || set.Timeout != 30*time.Second {
		t.Fatalf("%+v", set)
	}
	for _, c := range r.calls {
		if has(c, "VALUE") {
			t.Fatal("the value is on argv")
		}
	}
	// v1.4 spelling, probed once
	r = &scriptRunner{exit: func(c signing.StdioCall) (int, error) {
		if has(c, "config") {
			return 1, nil
		}
		return 0, nil
	}}
	st = &signing.CLIStore{Runner: r, Binary: "/bin/nself"}
	if err := st.Set(context.Background(), "N", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := st.Set(context.Background(), "N", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 4 || strings.Join(r.calls[1].Argv, " ") != "/bin/nself secrets set --help" || strings.Join(r.calls[2].Argv, " ") != "/bin/nself secrets set --env release N" {
		t.Fatalf("%d calls: %v", len(r.calls), r.calls)
	}
	// neither spelling
	r = &scriptRunner{exit: func(signing.StdioCall) (int, error) { return 1, nil }}
	if err := (&signing.CLIStore{Runner: r}).Probe(context.Background()); !signing.IsReason(err, signing.ReasonCLITooOld) {
		t.Fatalf("%v", err)
	}
	// the probe error path (runner cannot run) and a failing set
	r = &scriptRunner{exit: func(signing.StdioCall) (int, error) { return 0, errors.New("x") }}
	if err := (&signing.CLIStore{Runner: r}).Set(context.Background(), "N", []byte("v")); !signing.IsReason(err, signing.ReasonCLITooOld) {
		t.Fatalf("%v", err)
	}
	failing := &scriptRunner{exit: func(c signing.StdioCall) (int, error) {
		if c.Stdin != nil {
			return 3, nil
		}
		return 0, nil
	}}
	err := (&signing.CLIStore{Runner: failing}).Set(context.Background(), "N", []byte("SECRETVALUE"))
	if !signing.IsReason(err, signing.ReasonSignerFailed) || !strings.Contains(err.Error(), "exit 3") || strings.Contains(err.Error(), "SECRETVALUE") {
		t.Fatalf("%v", err)
	}
	// a runner error on set
	failing = &scriptRunner{exit: func(c signing.StdioCall) (int, error) {
		if c.Stdin != nil {
			return 0, errors.New("x")
		}
		return 0, nil
	}}
	if err := (&signing.CLIStore{Runner: failing}).Set(context.Background(), "N", []byte("v")); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("%v", err)
	}
}

func mustPub(t *testing.T, info signing.KeyInfo) []byte {
	t.Helper()
	b, err := base64Decode(info.Public)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
