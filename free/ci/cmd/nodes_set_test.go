package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestNodesSetCommand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		exit  int
		infra bool
	}{
		{"node first", []string{"lap1", "--reserve-cpu", "1"}, 0, false},
		{"flags first", []string{"--reserve-cpu", "1", "lap1"}, 0, false},
		{"json", []string{"lap1", "--reserve-cpu", "1", "--json"}, 0, false},
		{"no setting", []string{"lap1", "--json"}, 1, false},
		{"invalid value", []string{"lap1", "--interactive", "bad", "--json"}, 1, false},
		{"missing node", []string{"missing", "--reserve-cpu", "1", "--json"}, 1, false},
		{"store unavailable", []string{"lap1", "--reserve-cpu", "1", "--json"}, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			raw, err := os.ReadFile("../internal/model/testdata/capability/valid/laptop.valid.json")
			if err != nil {
				t.Fatal(err)
			}
			var c model.Capability
			if err = json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			c.Identity.ID, c.Identity.Name = "lap1", "lap1"
			if _, err = registry.New(st).Register(context.Background(), c, "cli:test"); err != nil {
				t.Fatal(err)
			}
			env := lifecycle.Env{Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
				if tc.infra {
					return nil, nil, errors.New("unavailable")
				}
				return lifecycle.NewDeps(st, "", "", "cli:test"), func() {}, nil
			}}
			var out, errs bytes.Buffer
			if got := runNodesSet(tc.args, env, &out, &errs); got != tc.exit {
				t.Fatalf("exit %d want %d: %s %s", got, tc.exit, &out, &errs)
			}
			if nodesSetHasJSON(tc.args) {
				var envelope map[string]json.RawMessage
				if err = json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatalf("invalid JSON: %s", &out)
				}
				key := "data"
				if tc.exit != 0 {
					key = "error"
				}
				if len(envelope[key]) == 0 || string(envelope[key]) == "null" {
					t.Fatalf("missing %s: %s", key, &out)
				}
			}
			saved, ok, err := st.NodeSettingsGet(context.Background(), "lap1")
			if err != nil {
				t.Fatal(err)
			}
			if tc.exit == 0 && (!ok || saved.ReservedCPU != 1) {
				t.Fatalf("settings %+v exist %v", saved, ok)
			}
			if tc.exit != 0 && ok {
				t.Fatal("failed command changed settings")
			}
		})
	}
}
