package local

// Purpose: test doubles for the provider: a fake Backend (a runtime with sandboxes, an outage switch and
//          failure injection) and a fake isolation profile for the two real backends.
// Constraints: no process, no network, no disk; the clock is injected.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

type fakeBackend struct {
	mu          sync.Mutex
	platform    string
	level       model.Isolation
	total       int
	running     []string
	down        bool
	boxes       map[string]Found
	prepared    []string
	torn        []string
	prepareErr  error
	teardownErr error
	listErr     error
}

func newFake() *fakeBackend {
	return &fakeBackend{platform: "linux/amd64", level: containerLevel, total: -1, boxes: map[string]Found{}}
}

func (f *fakeBackend) Platform() string { return f.platform }

func (f *fakeBackend) outage() error {
	if f.down {
		return unavailable(ReasonNoRuntime, nil, "runtime is down")
	}
	return nil
}

func (f *fakeBackend) Probe(context.Context) (Offer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.outage(); err != nil {
		return Offer{}, err
	}
	return Offer{Level: f.level, Total: f.total, Running: f.running}, nil
}

func (f *fakeBackend) Prepare(_ context.Context, a isolation.Attempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.outage(); err != nil {
		return err
	}
	if f.prepareErr != nil {
		return f.prepareErr
	}
	f.prepared = append(f.prepared, a.ID)
	return nil
}

func (f *fakeBackend) Teardown(_ context.Context, a isolation.Attempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.outage(); err != nil {
		return err
	}
	if f.teardownErr != nil {
		return f.teardownErr
	}
	f.torn = append(f.torn, a.ID)
	delete(f.boxes, a.ID)
	return nil
}

func (f *fakeBackend) Leftovers(context.Context) ([]Found, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.outage(); err != nil {
		return nil, err
	}
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []Found
	for _, b := range f.boxes {
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBackend) setDown(on bool) { f.mu.Lock(); f.down = on; f.mu.Unlock() }

func (f *fakeBackend) add(b Found) { f.mu.Lock(); f.boxes[b.ID] = b; f.mu.Unlock() }

var epoch = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// bigHost is a host no reservation limits.
func bigHost() (exec.Capacity, reservation.Settings) {
	return exec.Capacity{CPUs: 64, MemoryMB: 1 << 20, FreeMemoryMB: 1 << 20, BatteryPercent: -1}, reservation.Settings{}
}

func newProvider(t interface{ Fatal(...any) }, b Backend, mut func(*Options)) *Provider {
	o := Options{Backend: b, Host: bigHost, Now: func() time.Time { return epoch }}
	if mut != nil {
		mut(&o)
	}
	p, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeProfile is an isolation.Profile and Lister with scripted answers.
type fakeProfile struct {
	level    model.Isolation
	av       isolation.Availability
	names    []string
	listErr  error
	prepErr  error
	tearErr  error
	tornDown []string
}

func (p *fakeProfile) Level() model.Isolation                           { return p.level }
func (p *fakeProfile) Probe(context.Context) isolation.Availability     { return p.av }
func (p *fakeProfile) Prepare(context.Context, isolation.Attempt) error { return p.prepErr }
func (p *fakeProfile) Wrap(s exec.JobSpec) (exec.JobSpec, error)        { return s, nil }
func (p *fakeProfile) Teardown(_ context.Context, a isolation.Attempt) error {
	p.tornDown = append(p.tornDown, a.ID)
	return p.tearErr
}
func (p *fakeProfile) List(context.Context, string) ([]string, error) { return p.names, p.listErr }

var _ isolation.Lister = (*fakeProfile)(nil)
