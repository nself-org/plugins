package main

// Purpose: tests of the serve mode wiring (P7-NODE-20): the --team flag, the start check and the facts
// provider registration.
// Inputs:  temporary stores and flag sets built by newServeV15Flags.
// Outputs: pass or fail.
// Constraints: nothing is bound; the provider registry is process-wide, so tests only read it.

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/mode"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func modeStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestServeModeFlagAndStart(t *testing.T) {
	ctx := context.Background()
	if !world.CoordinatorFactsRegistered() {
		t.Fatal("the binary must register the coordinator-facts provider")
	}
	parse := func(args ...string) bool {
		t.Helper()
		fs, _, _ := newServeV15Flags()
		if fs.Lookup("team") == nil {
			t.Fatal("serve has no --team flag")
		}
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		return serveTeamFlag(fs)
	}
	if parse() || !parse("--team") {
		t.Fatal("--team parsing")
	}
	st := modeStore(t)
	var out bytes.Buffer
	fs, _, _ := newServeV15Flags()
	end, err := serveModeStart(ctx, fs, st, &out)
	if err != nil || !mode.Serving() || !strings.Contains(out.String(), "mode personal") {
		t.Fatalf("personal start: %v serving=%v %q", err, mode.Serving(), out.String())
	}
	if f, _ := mode.Facts(ctx); !f.Serving || f.Mode != mode.Personal {
		t.Fatalf("%+v", f)
	}
	end()
	if mode.Serving() {
		t.Fatal("the serve is still reported after end")
	}
	// --team switches once; a later start without the flag stays team
	fs, _, _ = newServeV15Flags()
	if err = fs.Parse([]string{"--team"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	end, err = serveModeStart(ctx, fs, st, &out)
	if err != nil || !strings.Contains(out.String(), "set to team") {
		t.Fatalf("team start: %v %q", err, out.String())
	}
	end()
	fs, _, _ = newServeV15Flags()
	if end, err = serveModeStart(ctx, fs, st, &out); err != nil || st.CoordinatorMode(ctx) != store.CoordinatorTeam {
		t.Fatalf("restart without the flag: %v", err)
	}
	end()
}

func TestServeModeRefusals(t *testing.T) {
	ctx := context.Background()
	fs, _, _ := newServeV15Flags()
	old := serveFactsRegistered
	serveFactsRegistered = func() bool { return false }
	t.Cleanup(func() { serveFactsRegistered = old })
	st := modeStore(t)
	if _, err := serveModeStart(ctx, fs, st, &bytes.Buffer{}); mode.ReasonOf(err) != mode.ReasonFactsProviderMissing || mode.Serving() {
		t.Fatalf("no provider must refuse the start: %v", err)
	}
	if _, found, _ := st.CoordinatorModeGet(ctx); found {
		t.Fatal("the refusal must come before the mode row is written")
	}
}
