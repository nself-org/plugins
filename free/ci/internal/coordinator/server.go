package coordinator

// Purpose: the V15 coordinator (P7-NODE-10, Epic D7): `nself ci serve` in V15 mode. It owns the durable
// intake, the dispatch loop, the Reconciler schedule and the listeners; the v1.4 serve stays frozen in
// internal/serve and only its verifier, allowlist and event parser are reused.
// Inputs:  Config (store, bind address, secret, allowlist, dispatcher, clock, guard) and the registries
//          (RegisterIngress, RegisterListener, RegisterFlag, RegisterReconciler).
// Outputs: Run blocks until ctx ends; Handler serves /healthz, /webhook, POST / and /v1/trigger.
// Constraints: the start guard (E663) and the secret check run before any port is bound; the default
//          fail-closed behaviour of v1.4 is unchanged (no secret, no start, unless --insecure); a
//          Dispatcher result is recorded, never lost: a cancelled dispatch returns the row to the queue.

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RunResult is what a Dispatcher reports for a pipeline it ran to the end (a failed gate is a result).
type RunResult struct {
	PipelineID string
	Exit       int
}

// Dispatcher runs the pipeline an intake row stands for. An error is an infrastructure failure.
type Dispatcher func(ctx context.Context, row store.IntakeRow) (RunResult, error)

// Clock is the time source of the schedules; tests inject a fake.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Config is the coordinator configuration.
type Config struct {
	Store        *store.Store
	Addr         string // webhook listen address
	Secret       string // HMAC secret for the v1.4 path
	Insecure     bool   // start without a secret (loud opt-out, as in v1.4)
	AllowedRepos []string
	Force        bool   // never coalesce: every accepted trigger runs (audited)
	PolicyDigest string // part of the semantic key
	Concurrency  int
	Dispatch     Dispatcher // nil: intake only
	Reconcilers  []Reconciler
	Listeners    map[string]string // listener name -> address; "" or absent skips an optional listener
	Guard        func(ctx context.Context) error
	Listen       func(network, addr string) (net.Listener, error)
	Clock        Clock
	Poll, Beat   time.Duration
	StaleAfter   time.Duration
	ID           string
	Logf         func(format string, a ...any)
}

// Coordinator is the TEAM coordinator.
type Coordinator struct {
	cfg  Config
	wake chan struct{}
	mu   sync.Mutex
	last map[string]time.Time
}

// New fills the defaults of cfg.
func New(cfg Config) (*Coordinator, error) {
	if cfg.Store == nil {
		return nil, newErr(CodeStore, ReasonStoreUnavailable, "no store")
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.Listen == nil {
		cfg.Listen = net.Listen
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Poll <= 0 {
		cfg.Poll = time.Second
	}
	if cfg.Beat <= 0 {
		cfg.Beat = 30 * time.Second
	}
	if cfg.StaleAfter <= 0 {
		cfg.StaleAfter = 3 * cfg.Beat
	}
	if cfg.ID == "" {
		cfg.ID = "coord-" + newToken()
	}
	return &Coordinator{cfg: cfg, wake: make(chan struct{}, 1), last: map[string]time.Time{}}, nil
}

func (c *Coordinator) nudge() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// ListenerStart binds and serves one extra listener; stop shuts it down.
type ListenerStart func(ctx context.Context, addr string, c *Coordinator) (stop func(context.Context) error, err error)

type listenerSpec struct {
	name, flag, def string
	start           ListenerStart
}

var registry struct {
	sync.Mutex
	listeners []listenerSpec
	flags     []func(*flag.FlagSet)
}

// RegisterListener adds a listener with its flag and default address (P7-NODE-19 and P7-NODE-20 call it
// from their own files). A repeated name or flag is refused.
func RegisterListener(name, flagName, def string, start ListenerStart) error {
	if name == "" || flagName == "" || start == nil {
		return newErr("", ReasonInvalidConfig, "listener needs a name, a flag and a start function")
	}
	registry.Lock()
	defer registry.Unlock()
	for _, l := range registry.listeners {
		if l.name == name || l.flag == flagName {
			return newErr("", ReasonDuplicateName, "listener %q (flag %q) is already registered", name, flagName)
		}
	}
	registry.listeners = append(registry.listeners, listenerSpec{name, flagName, def, start})
	return nil
}

// RegisterFlag adds serve flags that are not listeners; they are parsed before any listener binds.
func RegisterFlag(add func(*flag.FlagSet)) {
	registry.Lock()
	defer registry.Unlock()
	registry.flags = append(registry.flags, add)
}

// AddFlags registers the flag of every registered listener and every RegisterFlag function on fs and
// returns a function that reads the listener addresses after fs.Parse.
func AddFlags(fs *flag.FlagSet) (addrs func() map[string]string) {
	registry.Lock()
	specs := append([]listenerSpec(nil), registry.listeners...)
	extra := append([]func(*flag.FlagSet)(nil), registry.flags...)
	registry.Unlock()
	vals := map[string]*string{}
	for _, l := range specs {
		vals[l.name] = fs.String(l.flag, l.def, "Listen address of the "+l.name+" listener")
	}
	for _, f := range extra {
		f(fs)
	}
	return func() map[string]string {
		out := map[string]string{}
		for k, v := range vals {
			out[k] = *v
		}
		return out
	}
}

// checkSecret keeps the v1.4 refusal text and adds the per-repo secrets of registered ingresses.
func (c *Coordinator) checkSecret() error {
	if c.cfg.Secret != "" || c.cfg.Insecure || anySecrets(registeredIngresses()) {
		return nil
	}
	return newErr("", ReasonNoSecret, "refusing to start: no webhook secret configured (--secret or GITHUB_WEBHOOK_SECRET); pass --insecure to start anyway (NOT recommended — this exposes an unauthenticated arbitrary-repo code-execution endpoint)")
}

// Run starts the coordinator and blocks until ctx ends or a listener fails.
func (c *Coordinator) Run(ctx context.Context) error {
	if c.cfg.Guard != nil { // before anything is bound or read
		if err := c.cfg.Guard(ctx); err != nil {
			return err
		}
	}
	if err := c.checkSecret(); err != nil {
		return err
	}
	ln, err := c.cfg.Listen("tcp", c.cfg.Addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", c.cfg.Addr, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	srv := &http.Server{Handler: c.Handler(isLoopback(ln.Addr().String())), ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
	errc := make(chan error, 1)
	go func() {
		if e := srv.Serve(ln); e != nil && e != http.ErrServerClosed {
			errc <- e
		}
	}()
	stops, err := c.startListeners(ctx)
	if err != nil {
		_ = srv.Close()
		return err
	}
	var wg sync.WaitGroup
	for _, f := range []func(context.Context){c.maintain, c.dispatchLoop, c.runReconcilers} {
		wg.Add(1)
		go func(f func(context.Context)) { defer wg.Done(); f(ctx) }(f)
	}
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}
	cancel()
	sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
	defer scancel()
	_ = srv.Shutdown(sctx)
	for _, stop := range stops {
		_ = stop(sctx)
	}
	wg.Wait()
	return runErr
}

// startListeners starts every registered listener that has an address.
func (c *Coordinator) startListeners(ctx context.Context) ([]func(context.Context) error, error) {
	registry.Lock()
	specs := append([]listenerSpec(nil), registry.listeners...)
	registry.Unlock()
	sort.Slice(specs, func(i, j int) bool { return specs[i].name < specs[j].name })
	var stops []func(context.Context) error
	for _, l := range specs {
		addr, set := c.cfg.Listeners[l.name]
		if !set {
			addr = l.def
		}
		if strings.TrimSpace(addr) == "" {
			continue
		}
		stop, err := l.start(ctx, addr, c)
		if err != nil {
			for _, s := range stops {
				_ = s(ctx)
			}
			return nil, fmt.Errorf("listener %s: %w", l.name, err)
		}
		stops = append(stops, stop)
	}
	return stops, nil
}

// Handler serves the coordinator routes. loopback says whether the listener is bound to a loopback
// address: only then /healthz adds the queue depth and lease counts.
func (c *Coordinator) Handler(loopback bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { c.healthz(w, r, loopback) })
	mux.HandleFunc("/webhook", c.serveWebhook)
	mux.HandleFunc("/v1/trigger", c.serveTrigger)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path != "/":
			http.NotFound(w, r)
		case r.Method == http.MethodPost:
			c.serveWebhook(w, r)
		default:
			c.info(w)
		}
	})
	return accessLog(mux, c.cfg.Logf)
}
