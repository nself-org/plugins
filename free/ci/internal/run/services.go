package run

// Purpose: registers the disposable-services hook (P7-CI-42, Epic D9). The
// orchestrator calls Up before a job that needs services and Down after it;
// this adapter maps the request onto services.Provisioner.
// Inputs:  ServicesRequest (run id and service names). Service names resolve
// through the nearest .nself/ci.yaml above the working directory
// (`services: {<name>: {component: ...}}`); a name that is not declared there
// is taken as an nSelf component name (`--service postgres`).
// Outputs: the NSELF_CI_SERVICE_* environment of the started services.
// Constraints: the request carries no attempt id, so two jobs of one run that
// need the same services would be indistinguishable at Down. They are
// therefore serialised per (run, names): the second Up waits for the first
// Down, and each job still gets its own throwaway project. A job context that
// ends before Down tears the project down through a safety net; the slot stays
// held until the owning job's Down, so that Down cannot hit a later job.
// The engine runs only in v1.5 mode: cmd/main.go dispatches to this package
// only when compat.V15() is true.

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

func init() {
	RegisterServices(&servicesHook{slots: map[string]chan struct{}{}, live: map[string]*liveServices{}})
}

// liveServices is one running provision. Its teardown runs once, whoever asks
// first (the safety net or Down); every other caller waits for that run and
// sees its result, so Down never returns while a teardown is still going.
type liveServices struct {
	handle *services.Handle
	stop   func() bool // cancels the safety net
	once   sync.Once
	done   chan struct{}
	err    error
}

func (l *liveServices) teardown(ctx context.Context, p *services.Provisioner) error {
	l.once.Do(func() {
		l.err = p.Down(ctx, l.handle)
		close(l.done)
	})
	<-l.done
	return l.err
}

type servicesHook struct {
	once    sync.Once
	homeErr error // set once, so every Up after a failed lookup fails the same way
	prov    *services.Provisioner
	mu      sync.Mutex
	secrets []string
	slots   map[string]chan struct{}
	live    map[string]*liveServices
}

// Up starts the services of one job.
func (s *servicesHook) Up(ctx context.Context, r ServicesRequest) (ServicesSetup, error) {
	prov, err := s.provisioner(ctx)
	if err != nil {
		return ServicesSetup{}, err
	}
	key := serviceKey(r)
	if err := s.acquire(ctx, key); err != nil {
		return ServicesSetup{}, err
	}
	h, err := prov.Up(ctx, serviceSpecs(r.Names))
	if err != nil {
		s.release(key)
		return ServicesSetup{}, err
	}
	l := &liveServices{handle: h, done: make(chan struct{})}
	l.stop = context.AfterFunc(ctx, func() { s.safetyNet(l) })
	s.mu.Lock()
	s.live[key] = l
	s.mu.Unlock()
	return ServicesSetup{Env: h.Env}, nil
}

// Down tears the services of one job down and frees the request slot. It
// returns only when the teardown has finished, including one the safety net
// started.
func (s *servicesHook) Down(ctx context.Context, r ServicesRequest) error {
	key := serviceKey(r)
	s.mu.Lock()
	l := s.live[key]
	delete(s.live, key)
	s.mu.Unlock()
	if l == nil {
		return nil
	}
	l.stop()
	defer s.release(key)
	// The orchestrator's cleanup context is short (15 s); a teardown that is cut
	// off leaves containers behind, so it gets its own budget.
	bg, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer done()
	return l.teardown(bg, s.prov)
}

// safetyNet tears the services down when the job context ended before Down
// ran. It keeps the entry and the request slot: the owning job's Down is still
// coming and must find its own entry, not a later job's with the same key.
func (s *servicesHook) safetyNet(l *liveServices) {
	bg, done := context.WithTimeout(context.Background(), 2*time.Minute)
	defer done()
	_ = l.teardown(bg, s.prov)
}

// provisioner builds the engine once and sweeps the projects of dead
// coordinators before the first provision of this process.
func (s *servicesHook) provisioner(ctx context.Context) (*services.Provisioner, error) {
	s.once.Do(func() {
		home := os.Getenv("NSELF_CI_HOME")
		if home == "" {
			u, err := os.UserHomeDir()
			if err != nil {
				s.homeErr = err
				return
			}
			home = filepath.Join(u, ".nself", "ci")
		}
		run := &services.Funnel{Home: home, Secrets: s.secretList}
		s.prov = &services.Provisioner{Run: run, Home: home, Secret: s.addSecret}
		sctx, done := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer done()
		_, _ = s.prov.Sweep(sctx)
	})
	if s.homeErr != nil {
		return nil, &services.Error{Code: services.CodeProvision, Message: "cannot find the CI home: " + s.homeErr.Error()}
	}
	return s.prov, nil
}

func (s *servicesHook) addSecret(v string) {
	s.mu.Lock()
	s.secrets = append(s.secrets, v)
	s.mu.Unlock()
	if services.SecretSink != nil {
		services.SecretSink(v)
	}
}

func (s *servicesHook) secretList() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.secrets...)
}

func serviceKey(r ServicesRequest) string {
	names := append([]string(nil), r.Names...)
	sort.Strings(names)
	return r.RunID + "\x00" + strings.Join(names, ",")
}

// acquire waits for the slot of key, or for ctx.
func (s *servicesHook) acquire(ctx context.Context, key string) error {
	s.mu.Lock()
	ch, ok := s.slots[key]
	if !ok {
		ch = make(chan struct{}, 1)
		s.slots[key] = ch
	}
	s.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *servicesHook) release(key string) {
	s.mu.Lock()
	ch := s.slots[key]
	s.mu.Unlock()
	if ch != nil {
		select {
		case <-ch:
		default:
		}
	}
}

// serviceSpecs maps names to specs through the nearest .nself/ci.yaml.
func serviceSpecs(names []string) []services.Spec {
	declared := declaredServices()
	seen := map[string]bool{}
	var specs []services.Spec
	for _, n := range names {
		if seen[n] {
			continue
		}
		seen[n] = true
		sp := services.Spec{Name: n}
		if d, ok := declared[n]; ok {
			sp.Component, sp.Version = d.Component, d.Version
		}
		specs = append(specs, sp)
	}
	return specs
}

// declaredServices reads the services block of the nearest config, if any.
func declaredServices() map[string]struct{ Component, Version string } {
	out := map[string]struct{ Component, Version string }{}
	dir, err := os.Getwd()
	if err != nil {
		return out
	}
	for {
		if l, err := config.Load(filepath.Join(dir, config.FileName)); err == nil {
			for n, sv := range l.Config.Services {
				out[n] = struct{ Component, Version string }{sv.Component, sv.Version}
			}
			return out
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		dir = parent
	}
}
