package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// setEnv returns a nodes-set environment over a fresh store holding one registered node "box".
func setEnv(t *testing.T) (lifecycle.Env, string) {
	t.Helper()
	home := stubNself(t)
	putNode(t, home, draft(t, "box", "203.0.113.20"))
	return lifecycle.ProductionEnv(func(context.Context) string { return "proj" }), home
}

func runSet(t *testing.T, env lifecycle.Env, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = runNodesSet(args, env, &o, &e)
	return code, o.String(), e.String()
}

func storedSettings(t *testing.T, home string) store.NodeSettings {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	s, ok, err := st.NodeSettingsGet(context.Background(), "box")
	if err != nil || !ok {
		t.Fatalf("settings: ok=%v err=%v", ok, err)
	}
	return s
}

func TestNodesSetNeedsAtLeastOneFlag(t *testing.T) {
	env, _ := setEnv(t)
	for _, args := range [][]string{{"box"}, {"box", "--json"}, {"box", "--project", "proj"}} {
		code, out, errOut := runSet(t, env, args...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1 (stdout %q stderr %q)", args, code, out, errOut)
		}
		if !strings.Contains(out+errOut, "at least one of") {
			t.Errorf("%v: no at-least-one message: %q %q", args, out, errOut)
		}
	}
}

func TestNodesSetFlagOrderAndNodeIDRules(t *testing.T) {
	env, home := setEnv(t)
	// The node id may come first or last; flags before and after it parse the same.
	for _, args := range [][]string{
		{"box", "--reserve-cpu", "2", "--interactive", "ignore"},
		{"--reserve-cpu", "2", "--interactive", "ignore", "box"},
	} {
		if code, out, errOut := runSet(t, env, args...); code != 0 {
			t.Fatalf("%v: exit %d (stdout %q stderr %q)", args, code, out, errOut)
		}
		s := storedSettings(t, home)
		if s.ReservedCPU != 2 || s.Interactive != "ignore" {
			t.Errorf("%v: stored %+v", args, s)
		}
	}
	for _, args := range [][]string{{}, {"box", "other", "--reserve-cpu", "1"}, {"--reserve-cpu", "1"}} {
		if code, _, _ := runSet(t, env, args...); code != 1 {
			t.Errorf("%v: exit %d, want 1 (exactly one node id)", args, code)
		}
	}
	if code, _, _ := runSet(t, env, "bad id!", "--reserve-cpu", "1"); code != 1 {
		t.Errorf("invalid node id: exit %d, want 1", code)
	}
}

func TestNodesSetKeepsUnsetFlagsAndRejectsBadValues(t *testing.T) {
	env, home := setEnv(t)
	if code, _, e := runSet(t, env, "box", "--reserve-mem", "2g", "--on-battery", "drain"); code != 0 {
		t.Fatalf("first set: exit %d %s", code, e)
	}
	if code, _, e := runSet(t, env, "box", "--persistence", "intermittent"); code != 0 {
		t.Fatalf("second set: exit %d %s", code, e)
	}
	s := storedSettings(t, home)
	if s.ReservedMemMB != 2048 || s.OnBattery != "drain" || s.Persistence != "intermittent" {
		t.Errorf("a flag not given must keep its stored value: %+v", s)
	}
	for _, args := range [][]string{
		{"box", "--interactive", "sometimes"}, {"box", "--on-battery", "explode"},
		{"box", "--reserve-cpu", "lots"}, {"box", "--unknown-flag"},
	} {
		if code, _, _ := runSet(t, env, args...); code != 1 {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
	}
}

func TestNodesSetJSONEnvelopeAndExitCodes(t *testing.T) {
	env, _ := setEnv(t)
	code, out, errOut := runSet(t, env, "box", "--reserve-cpu", "1", "--json")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	var ok struct {
		Command string `json:"command"`
		Data    struct {
			Settings struct {
				NodeID      string  `json:"node"`
				ReservedCPU float64 `json:"reserved_cpu"`
			} `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &ok); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if ok.Command != "ci nodes set" || ok.Data.Settings.NodeID != "box" || ok.Data.Settings.ReservedCPU != 1 {
		t.Errorf("envelope %+v", ok)
	}
	// A usage error under --json is an error envelope on stdout with exit 1 (user error).
	code, out, _ = runSet(t, env, "box", "--json")
	var bad struct {
		Error struct {
			Code     string `json:"code"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &bad); err != nil || code != 1 || bad.Error.Code != "E651" || bad.Error.ExitCode != 1 {
		t.Errorf("usage envelope: exit %d err %v %+v\n%s", code, err, bad, out)
	}
	// An unknown node is a user error too; an unopenable store is an infrastructure error (exit 2).
	if code, _, _ = runSet(t, env, "ghost", "--reserve-cpu", "1"); code != 1 {
		t.Errorf("unknown node: exit %d, want 1", code)
	}
	broken := lifecycle.Env{Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
		return nil, nil, context.DeadlineExceeded
	}}
	code, out, _ = runSet(t, broken, "box", "--reserve-cpu", "1", "--json")
	if code != 2 || !strings.Contains(out, `"E607"`) {
		t.Errorf("store failure: exit %d out %s", code, out)
	}
}
