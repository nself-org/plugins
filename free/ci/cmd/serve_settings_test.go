package main

import (
	"context"
	"flag"
	"testing"
)

func TestServeSettingsSnapshot(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		fs.String("data-dir", "first", "")
		fs.String("api-socket", "auto", "")
		if explicit {
			if err := fs.Set("api-socket", "chosen"); err != nil {
				t.Fatal(err)
			}
		}
		ctx := withServeSettings(context.Background(), fs)
		if err := fs.Set("data-dir", "second"); err != nil {
			t.Fatal(err)
		}
		withServeSettings(context.Background(), fs)
		if got := serveFlagContext(ctx, "data-dir"); got != "first" {
			t.Fatalf("snapshot changed: %q", got)
		}
		if got := settingsFrom(ctx).explicit["api-socket"]; got != explicit {
			t.Fatalf("explicit %v, want %v", got, explicit)
		}
	}
}
