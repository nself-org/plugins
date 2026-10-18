package services

// Purpose: Sweep ends a dead owner's CLI calls before it tears the project
// down, and keeps the project when a call cannot be ended (P7-CI-42 review).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// killerStub is a stub Runner that also implements the Funnel's KillStale.
type killerStub struct {
	*stub
	err    error
	mu     sync.Mutex
	events []string
}

func (k *killerStub) Run(ctx context.Context, c Call) (Result, error) {
	k.mu.Lock()
	k.events = append(k.events, "run:"+strings.Join(c.Argv, " "))
	k.mu.Unlock()
	return k.stub.Run(ctx, c)
}

func (k *killerStub) KillStale(_ context.Context, dead func(string) bool) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if dead("nselfci-dddddddd") {
		k.events = append(k.events, "kill:nselfci-dddddddd")
		return k.err
	}
	return nil
}

func deadProject(t *testing.T, p *Provisioner, name string) {
	t.Helper()
	work := filepath.Join(p.Home, "work")
	if err := os.MkdirAll(filepath.Join(work, name, "svc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, name+".lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSweepEndsCallsBeforeTeardown(t *testing.T) {
	k := &killerStub{stub: &stub{orphan: true}}
	p := newProv(t, k.stub)
	p.Run = k
	deadProject(t, p, "nselfci-dddddddd")
	if n, err := p.Sweep(context.Background()); err != nil || n != 1 {
		t.Fatalf("swept %d, err %v", n, err)
	}
	killed, tore := -1, -1
	for i, e := range k.events {
		if e == "kill:nselfci-dddddddd" && killed < 0 {
			killed = i
		}
		if strings.Contains(e, "compose -p nselfci-dddddddd down") && tore < 0 {
			tore = i
		}
	}
	if killed < 0 || tore < 0 || killed > tore {
		t.Fatalf("the calls must be ended before the teardown (kill %d, teardown %d):\n%s", killed, tore, strings.Join(k.events, "\n"))
	}
}

func TestSweepKeepsAProjectWhoseCallsSurvive(t *testing.T) {
	k := &killerStub{stub: &stub{orphan: true}, err: errors.New("stale process group survived")}
	p := newProv(t, k.stub)
	p.Run = k
	deadProject(t, p, "nselfci-dddddddd")
	n, err := p.Sweep(context.Background())
	if err == nil || n != 0 {
		t.Fatalf("swept %d, err %v: the teardown must be refused", n, err)
	}
	for _, e := range k.events {
		if strings.Contains(e, "compose -p") || strings.Contains(e, "stop --volumes") {
			t.Fatalf("tore the project down although a call survived: %s", e)
		}
	}
	if _, err := os.Stat(filepath.Join(p.Home, "work", "nselfci-dddddddd.lock")); err != nil {
		t.Fatal("the lock file must stay so the next Sweep retries")
	}
}
