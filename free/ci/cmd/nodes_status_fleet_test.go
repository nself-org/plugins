package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/fleet"
	"github.com/nself-org/plugins/free/ci/internal/nodes/probe"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type statusClock struct{ now time.Time }

func (c statusClock) Now() time.Time { return c.now }

// TestNodesStatusLocalJSON: `--local --json` is the demand file plus source: live.
func TestNodesStatusLocalJSON(t *testing.T) {
	home := t.TempDir()
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	doc := demand.Document{Queued: 2, Held: 1, Running: 1, Ready: false, ReadyWhy: []string{"disk below floor"},
		Watcher: demand.Watcher{Alive: true, LastPass: at.Format(time.RFC3339), IntervalSec: 30}}
	if err := demand.Write(demand.WriterConfig{Dir: home, Host: "mm4", Clock: statusClock{at}}, doc); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runNodesStatus([]string{"--local", "--json"}, nodesStatusDeps{Home: home}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	raw, err := os.ReadFile(filepath.Join(home, "demand.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	want["source"] = "live"
	if !reflect.DeepEqual(env.Data, want) {
		t.Fatalf("data = %v\nwant %v", env.Data, want)
	}
	out.Reset()
	code := runNodesStatus([]string{"--local", "--json"}, nodesStatusDeps{Home: t.TempDir()}, &out, io.Discard)
	env.Data = nil
	if err := json.Unmarshal(out.Bytes(), &env); err != nil || code != 0 || env.Data["source"] != "none" {
		t.Fatalf("no demand file: exit %d %v %s", code, err, out.String())
	}
	out.Reset()
	if code := runNodesStatus([]string{"--local"}, nodesStatusDeps{Home: home}, &out, io.Discard); code != 0 || !strings.Contains(out.String(), "why: disk below floor") {
		t.Fatalf("text: exit %d %s", code, out.String())
	}
}

func TestNodesStatusFleet(t *testing.T) {
	home := t.TempDir()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	statusRegister(t, st, "a1", "b2")
	_ = st.Close()
	d := nodesStatusDeps{Home: home, Open: openStatusStore,
		Reader: func(context.Context, string) fleet.Reader {
			return func(_ context.Context, n registry.Node) (probe.StatusDocs, error) {
				if n.Record.ID == "b2" {
					return probe.StatusDocs{}, errors.New("connection refused")
				}
				return probe.StatusDocs{}, nil
			}
		}}
	var out, errOut bytes.Buffer
	if code := runNodesStatus([]string{"--json"}, d, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	var env struct {
		Data fleet.Report `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Nodes) != 2 || env.Data.Unreachable != 1 || env.Data.Nodes[0].Watcher != fleet.WatcherNone || env.Data.Nodes[1].Unreachable != "connection refused" {
		t.Fatalf("report = %+v", env.Data)
	}
	out.Reset()
	if code := runNodesStatus(nil, d, &out, &errOut); code != 0 || !strings.Contains(out.String(), "b2: unreachable: connection refused") {
		t.Fatalf("table: exit %d %s", code, out.String())
	}
	out.Reset()
	empty := nodesStatusDeps{Home: t.TempDir(), Open: openStatusStore, Reader: d.Reader}
	if code := runNodesStatus(nil, empty, &out, &errOut); code != 0 || !strings.Contains(out.String(), "no nodes registered") {
		t.Fatalf("no store: exit %d %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(empty.Home, "state.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("status created a state database")
	}
	if code := runNodesStatus([]string{"extra"}, d, io.Discard, io.Discard); code != 1 {
		t.Fatalf("positional arg: exit %d, want 1", code)
	}
}

func TestNodesStatusRegistration(t *testing.T) {
	e, ok := v15Subcommands["nodes status"]
	if !ok || !e.V15Only || e.Flags == nil {
		t.Fatalf("nodes status entry = %+v", e)
	}
	for _, name := range []string{"json", "local"} {
		if e.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s missing", name)
		}
	}
}

func statusRegister(t *testing.T, st *store.Store, ids ...string) {
	t.Helper()
	raw, err := os.ReadFile("../internal/model/testdata/capability/valid/vps.valid.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		var c map[string]any
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		ident := c["identity"].(map[string]any)
		ident["id"], ident["name"] = id, id
		doc, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		rec := store.NodeRecord{ID: id, Name: id, State: "online", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: 1}
		if err := st.NodePut(context.Background(), rec, "test"); err != nil {
			t.Fatal(err)
		}
	}
}
