package main

import (
	"flag"
	"io"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/update"
)

func TestNodeCoordinatorHelp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags func() *flag.FlagSet
	}{
		{"set", func() *flag.FlagSet { return nodesSetFlagSet(&nodesSetFlags{}, io.Discard) }},
		{"upgrade", func() *flag.FlagSet { return update.FlagSet(io.Discard) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := tc.flags()
			f := fs.Lookup("coordinator")
			if f == nil || f.Usage == "" || f.DefValue != "" {
				t.Fatalf("missing coordinator help: %+v", f)
			}
			if err := fs.Parse([]string{"--coordinator", "https://coordinator.example"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
