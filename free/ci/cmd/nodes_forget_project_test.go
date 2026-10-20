package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestNodesForgetProject(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		exit int
	}{
		{"missing root", nil, 1}, {"extra root", []string{"/old", "/other"}, 1},
		{"forget", []string{"/old"}, 0}, {"json", []string{"--json", "/old"}, 0},
		{"flags after root", []string{"/old", "--json"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			ctx := context.Background()
			g := deployguard.New(st, "", "cli:test")
			if err = g.Seen(ctx, "/old"); err != nil {
				t.Fatal(err)
			}
			if err = g.Seen(ctx, "/keep"); err != nil {
				t.Fatal(err)
			}
			env := lifecycle.Env{Open: func(context.Context, string) (*lifecycle.Deps, func(), error) {
				return lifecycle.NewDeps(st, "", "", "cli:test"), func() {}, nil
			}}
			var out, errs bytes.Buffer
			if got := runNodesForgetProject(tc.args, env, &out, &errs); got != tc.exit {
				t.Fatalf("exit %d want %d: %s", got, tc.exit, &errs)
			}
			roots, err := st.ExclusionProjects(ctx)
			want := 2
			if tc.exit == 0 {
				want = 1
			}
			if err != nil || len(roots) != want {
				t.Fatalf("roots %v err %v", roots, err)
			}
			if tc.exit == 0 && roots[0] != "/keep" {
				t.Fatal(roots)
			}
			if tc.exit == 0 && len(tc.args) == 2 && !bytes.Contains(out.Bytes(), []byte(`"data"`)) {
				t.Fatal(out.String())
			}
		})
	}
}
