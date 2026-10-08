package main

import (
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat"
)

func TestV15OnlyFixture(t *testing.T) {
	const key = "run"
	old := subcommands[key]
	subcommands[key] = func([]string) int { return 14 }
	registerV15(key, subcommandEntry{Handler: func([]string) int { return 15 }, V15Only: true})
	defer func() { delete(v15Subcommands, key); subcommands[key] = old }()
	t.Setenv("NSELF_V15", "1")
	if !compat.V15() {
		t.Fatal("v1.5 fixture not enabled")
	}
	if got := runV15([]string{key}); got != 15 {
		t.Fatalf("v1.5 result %d", got)
	}
	t.Setenv("NSELF_V15", "0")
	if compat.V15() {
		t.Fatal("v1.4 fixture still enabled")
	}
	if got := run([]string{key}); got != 14 {
		t.Fatalf("v1.4 result %d", got)
	}
	t.Setenv("NSELF_V15", "1")
	if runV15(nil) != 2 {
		t.Fatal("bare v1.5 must return E603 class")
	}
}
