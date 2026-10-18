package run

// Purpose: tests of the services hook adapter (P7-CI-42): per-key
// serialisation, release on Down, the context safety net, and the mapping of
// service names to components. A stub Runner stands in for nself and docker.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/services"
)

type okRunner struct {
	mu    sync.Mutex
	calls []string
}

func (r *okRunner) Run(_ context.Context, c services.Call) (services.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line := strings.Join(c.Argv, " ")
	r.calls = append(r.calls, line)
	if i := strings.Index(line, " init --non-interactive --name "); i >= 0 {
		name := line[i+len(" init --non-interactive --name "):]
		_ = os.WriteFile(filepath.Join(c.Dir, ".env"), []byte("PROJECT_NAME="+name+"\n"), 0o600)
	}
	if strings.HasSuffix(line, "config services list --json") {
		return services.Result{Output: `[{"service":"redis","enabled":false}]`}, nil
	}
	return services.Result{}, nil
}

func (r *okRunner) saw(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func testHook(t *testing.T) (*servicesHook, *okRunner) {
	t.Helper()
	r := &okRunner{}
	h := &servicesHook{slots: map[string]chan struct{}{}, live: map[string]*liveServices{}}
	h.once.Do(func() {})
	h.prov = &services.Provisioner{Run: r, Home: t.TempDir(), Ready: func(context.Context, string) error { return nil }}
	return h, r
}

func TestServicesHookSerialisesIdenticalRequests(t *testing.T) {
	h, _ := testHook(t)
	req := ServicesRequest{RunID: "r1", Names: []string{"postgres"}}
	ctx := context.Background()
	if _, err := h.Up(ctx, req); err != nil {
		t.Fatal(err)
	}
	second := make(chan error, 1)
	go func() { _, err := h.Up(ctx, req); second <- err }()
	select {
	case err := <-second:
		t.Fatalf("the second Up must wait for the first Down, returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	// A different request of the same run is not held back.
	other := ServicesRequest{RunID: "r1", Names: []string{"postgres", "redis"}}
	if _, err := h.Up(ctx, other); err != nil {
		t.Fatalf("different names must not wait: %v", err)
	}
	if err := h.Down(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := h.Down(ctx, req); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second Up after Down: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second Up never proceeded after Down")
	}
	if err := h.Down(ctx, req); err != nil {
		t.Fatal(err)
	}
}

func TestServicesHookWaitEndsWithContext(t *testing.T) {
	h, _ := testHook(t)
	req := ServicesRequest{RunID: "r2", Names: []string{"postgres"}}
	if _, err := h.Up(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := h.Up(ctx, req); err == nil {
		t.Fatal("a waiting Up must fail when its context ends")
	}
	if err := h.Down(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestServicesHookSafetyNetTearsDownAndKeepsSlotUntilDown(t *testing.T) {
	h, r := testHook(t)
	req := ServicesRequest{RunID: "r3", Names: []string{"postgres"}}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := h.Up(ctx, req); err != nil {
		t.Fatal(err)
	}
	cancel() // the job context ends before Down
	deadline := time.Now().Add(5 * time.Second)
	for !r.saw("stop --volumes --remove-orphans") {
		if time.Now().After(deadline) {
			t.Fatal("no teardown after the context ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The slot is still held: a later job must not start before the owner's Down,
	// or that Down would find the later job's entry.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	if _, err := h.Up(ctx2, req); err == nil {
		t.Fatal("the slot was released by the safety net before Down")
	}
	if err := h.Down(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	if _, err := h.Up(ctx3, req); err != nil {
		t.Fatalf("slot not released by Down: %v", err)
	}
	_ = h.Down(context.Background(), req)
}

// blockRunner holds the `stop` call until released.
type blockRunner struct {
	okRunner
	started chan struct{}
	release chan struct{}
}

func (b *blockRunner) Run(ctx context.Context, c services.Call) (services.Result, error) {
	if strings.Contains(strings.Join(c.Argv, " "), "stop --volumes") {
		select {
		case b.started <- struct{}{}:
		default:
		}
		<-b.release
	}
	return b.okRunner.Run(ctx, c)
}

func TestServicesHookDownWaitsForARunningSafetyNet(t *testing.T) {
	b := &blockRunner{started: make(chan struct{}, 1), release: make(chan struct{})}
	h := &servicesHook{slots: map[string]chan struct{}{}, live: map[string]*liveServices{}}
	h.once.Do(func() {})
	h.prov = &services.Provisioner{Run: b, Home: t.TempDir(), Ready: func(context.Context, string) error { return nil }}
	req := ServicesRequest{RunID: "r4", Names: []string{"postgres"}}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := h.Up(ctx, req); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-b.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the safety net never started")
	}
	down := make(chan error, 1)
	go func() { down <- h.Down(context.Background(), req) }()
	select {
	case err := <-down:
		t.Fatalf("Down returned while the teardown was still running: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(b.release)
	select {
	case err := <-down:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Down never returned after the teardown finished")
	}
}

func TestServiceSpecsUseDeclaredComponents(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "version: 1\nservices:\n  db:\n    component: postgres\n    version: 16-alpine\n"
	if err := os.WriteFile(filepath.Join(dir, ".nself", "ci.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	got := serviceSpecs([]string{"db", "redis", "db"})
	if len(got) != 2 {
		t.Fatalf("duplicates must collapse: %+v", got)
	}
	if got[0] != (services.Spec{Name: "db", Component: "postgres", Version: "16-alpine"}) {
		t.Fatalf("declared service not mapped: %+v", got[0])
	}
	if got[1] != (services.Spec{Name: "redis"}) {
		t.Fatalf("an undeclared name is a component name: %+v", got[1])
	}
}
