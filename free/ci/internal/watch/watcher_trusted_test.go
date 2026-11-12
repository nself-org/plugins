package watch

import (
	"context"
	"testing"
)

func TestWire10EmptyTrustedRejected(t *testing.T) {
	r := newRig(t)
	c := Defaults()
	c.Repos, c.Host, c.Hosts, c.Trusted = []string{"o/r"}, "a", []string{"a"}, nil
	p := Ports{Scanner: r.scan, Ledger: r.gh.on("a"), Reporter: r.gh.on("a"), Runner: r.runner,
		Ready: func(context.Context) (bool, []string) { return true, nil }}
	before := len(r.gh.posts) + r.gh.claims
	w, err := New(c, p)
	if err == nil || w != nil {
		t.Fatalf("New with no trusted creators = %v, %v; want an error", w, err)
	}
	if n := len(r.gh.posts) + r.gh.claims; n != before {
		t.Fatalf("posted %d statuses, want none", n-before)
	}
}
