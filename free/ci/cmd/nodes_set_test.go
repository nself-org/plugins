package main

// Purpose: cmd-level tests of `nodes set` (D-1229): the node id before or after the flags, the
// at-least-one-flag rule, the --json envelope and the exit codes, through runNodesSet with an injected store.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type setRig struct {
	t  *testing.T
	st *store.Store
}

func newSetRig(t *testing.T) *setRig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err = registry.New(st).Register(context.Background(), *draft(t, "setnode", "set.example.test"), "operator"); err != nil {
		t.Fatal(err)
	}
	return &setRig{t: t, st: st}
}

// run executes `nodes set args...` and returns the exit code, stdout and stderr.
func (r *setRig) run(args ...string) (int, string, string) {
	r.t.Helper()
	var out, errb bytes.Buffer
	env := lifecycle.Env{Out: &out, Err: &errb, Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
		return lifecycle.NewDeps(r.st, r.t.TempDir(), "proj-1", "cli:test"), func() {}, nil
	}}
	code := runNodesSet(args, env, &out, &errb)
	return code, out.String(), errb.String()
}

func (r *setRig) stored() store.NodeSettings {
	r.t.Helper()
	s, ok, err := r.st.NodeSettingsGet(context.Background(), "setnode")
	if err != nil || !ok {
		r.t.Fatalf("settings not stored: ok=%v err=%v", ok, err)
	}
	return s
}

func TestNodesSetFlagsMayFollowOrPrecedeTheNodeAndKeepWhatIsNotGiven(t *testing.T) {
	r := newSetRig(t)
	if code, out, errs := r.run("setnode", "--reserve-cpu", "2", "--on-battery", "allow"); code != 0 || errs != "" || !strings.Contains(out, "node setnode: reserve cpu 2,") || !strings.Contains(out, "on battery allow") {
		t.Fatalf("node first: code=%d out=%q err=%q", code, out, errs)
	}
	if code, _, errs := r.run("--reserve-mem", "2048", "setnode"); code != 0 {
		t.Fatalf("flags first: code=%d err=%q", code, errs)
	}
	got := r.stored()
	if got.ReservedCPU != 2 || got.ReservedMemMB != 2048 || got.OnBattery != store.OnBatteryAllow {
		t.Fatalf("a flag not given must keep its stored value: %+v", got)
	}
}

func TestNodesSetNeedsAtLeastOneFlag(t *testing.T) {
	r := newSetRig(t)
	code, out, errs := r.run("setnode")
	if code != 1 || out != "" || !strings.Contains(errs, "needs at least one of --reserve-cpu") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	if _, ok, _ := r.st.NodeSettingsGet(context.Background(), "setnode"); ok {
		t.Fatal("a refused call stored settings")
	}
	code, out, _ = r.run("setnode", "--json")
	var env struct {
		Command string `json:"command"`
		Error   struct {
			Code     string `json:"code"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != 1 || env.Command != "ci nodes set" || env.Error.Code != "E651" || env.Error.ExitCode != 1 {
		t.Fatalf("json refusal: code=%d env=%+v err=%v out=%q", code, env, err, out)
	}
}

func TestNodesSetJSONEnvelopeCarriesTheStoredSettings(t *testing.T) {
	r := newSetRig(t)
	code, out, errs := r.run("setnode", "--reserve-mem", "1024", "--persistence", "intermittent", "--json")
	if code != 0 || errs != "" {
		t.Fatalf("code=%d err=%q out=%q", code, errs, out)
	}
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Data          struct {
			Settings store.NodeSettings `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	s := env.Data.Settings
	if env.SchemaVersion != "1" || env.Command != "ci nodes set" || s.NodeID != "setnode" || s.ReservedMemMB != 1024 || s.Persistence != store.PersistenceInterm {
		t.Fatalf("envelope = %+v", env)
	}
}

func TestNodesSetExitCodes(t *testing.T) {
	r := newSetRig(t)
	for name, tc := range map[string]struct {
		args []string
		code int
		msg  string
	}{
		"no node":         {[]string{"--reserve-cpu", "1"}, 1, "usage: nself ci nodes set <node>"},
		"two nodes":       {[]string{"setnode", "other", "--reserve-cpu", "1"}, 1, "usage: nself ci nodes set <node>"},
		"unknown flag":    {[]string{"setnode", "--nope"}, 1, "nope"},
		"bad enum":        {[]string{"setnode", "--on-battery", "sideways"}, 1, "on_battery must be pause, drain or allow"},
		"whole machine":   {[]string{"setnode", "--reserve-cpu", "8"}, 1, "refuse all CI work"},
		"bad node id":     {[]string{"../x", "--reserve-cpu", "1"}, 1, "node id is not valid"},
		"unregistered":    {[]string{"ghost", "--reserve-cpu", "1"}, 1, "E650"},
		"unknown, --json": {[]string{"ghost", "--reserve-cpu", "1", "--json"}, 1, `"code": "E650"`},
	} {
		code, out, errs := r.run(tc.args...)
		if code != tc.code || !strings.Contains(out+errs, tc.msg) {
			t.Errorf("%s: code=%d want %d, out=%q err=%q, want %q", name, code, tc.code, out, errs, tc.msg)
		}
	}
	if _, ok, _ := r.st.NodeSettingsGet(context.Background(), "setnode"); ok {
		t.Fatal("a refused call stored settings")
	}
}

func TestNodesSetStoreFailureIsAnInfrastructureExit(t *testing.T) {
	var out, errb bytes.Buffer
	env := lifecycle.Env{Out: &out, Err: &errb, Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
		return nil, nil, errors.New("disk on fire")
	}}
	if code := runNodesSet([]string{"setnode", "--reserve-cpu", "1"}, env, &out, &errb); code != 2 || !strings.Contains(errb.String(), "the CI store cannot be opened") {
		t.Fatalf("code=%d err=%q", code, errb.String())
	}
}
