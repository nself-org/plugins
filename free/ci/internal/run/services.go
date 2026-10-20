package run

// Purpose: registers the disposable-services hook (P7-CI-42, Epic D9). The
// orchestrator calls Up before a job that needs services and Down after it;
// this adapter maps the request onto services.Pool and services.Provisioner.
// Inputs:  ServicesRequest (attempt id and service names or resolved specs).
// Names resolve through the nearest .nself/ci.yaml above the repository root
// (`services: {<name>: {component: ...}}`); a name that is not declared there
// is taken as an nSelf component name (`--service postgres`).
// Outputs: the NSELF_CI_SERVICE_* environment of the started services.
// Constraints: the lifecycle (one provision per attempt, safety net, one-shot
// teardown) lives in services.Pool, where it is tested. The engine runs only
// in v1.5 mode: cmd/main.go dispatches to this package only when compat.V15()
// is true.

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/config"
	"github.com/nself-org/plugins/free/ci/internal/services"
)

func init() {
	RegisterServices(&servicesHook{pool: services.NewPool()})
}

type servicesHook struct {
	once    sync.Once
	homeErr error // set once, so every Up after a failed lookup fails the same way
	prov    *services.Provisioner
	pool    *services.Pool
	mu      sync.Mutex
	secrets []string
}

// Up starts the services of one job.
func (s *servicesHook) Up(ctx context.Context, r ServicesRequest) (ServicesSetup, error) {
	prov, err := s.provisioner(ctx)
	if err != nil {
		return ServicesSetup{}, err
	}
	key, err := serviceKey(r)
	if err != nil {
		return ServicesSetup{}, err
	}
	specs := r.Specs
	if len(specs) == 0 {
		specs = ServiceSpecs(r.Names)
	}
	env, err := s.pool.Up(ctx, prov, key, specs)
	if err != nil {
		return ServicesSetup{}, err
	}
	return ServicesSetup{Env: env}, nil
}

// Down tears the services of one job down.
func (s *servicesHook) Down(ctx context.Context, r ServicesRequest) error {
	key, err := serviceKey(r)
	if err != nil {
		return err
	}
	return s.pool.Down(ctx, key)
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

// serviceKey is the pool key of a request: its attempt, so every attempt has its own provision and its own Down.
func serviceKey(r ServicesRequest) (string, error) {
	if r.AttemptID == "" {
		return "", &services.Error{Code: services.CodeProvision, Message: "a services request needs an attempt id"}
	}
	return r.AttemptID, nil
}

// ServiceSpecs maps names to specs through the nearest .nself/ci.yaml above the working directory.
func ServiceSpecs(names []string) []services.Spec { return ServiceSpecsAt("", names) }

// ServiceSpecsAt is ServiceSpecs reading the nearest .nself/ci.yaml above dir ("" is the working directory).
func ServiceSpecsAt(dir string, names []string) []services.Spec {
	declared := declaredServices(dir)
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

// declaredServices reads the services block of the nearest config above dir, if any.
func declaredServices(dir string) map[string]struct{ Component, Version string } {
	out := map[string]struct{ Component, Version string }{}
	if dir == "" {
		var err error
		if dir, err = os.Getwd(); err != nil {
			return out
		}
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
