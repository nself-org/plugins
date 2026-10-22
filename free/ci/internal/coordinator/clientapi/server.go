package clientapi

// Purpose: the coordinator user API (P7-NODE-19, Epic D14): the HTTP handler behind `--api-addr` and the
// local unix socket. Every route, built in or registered by another Ticket, goes through one wrapper that
// authenticates the caller (user certificate or scoped token over TLS; the operator on the local socket),
// checks the route's role and writes E664 on refusal.
// Inputs:  Config (Backend, role Service, repo allowlist, mode) and the RegisterRoute registry.
// Outputs: an http.Handler; JSON bodies; NDJSON for events and logs.
// Constraints: a node or coordinator certificate is never accepted (E664 node_kind); a token only over TLS;
//          no handler reads a project, realm or role from a header; a response never carries a token, a
//          certificate or a store error text; a refusal of a run the caller may not see looks like not found.

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// Config configures a Server.
type Config struct {
	Backend    Backend
	Roles      *roles.Service
	Allow      func(project string) bool // the coordinator's repo allowlist; nil allows no project (fail closed; Local mode ignores it)
	Local      bool                      // unix-socket mode: the caller is the operator, no credential is read
	Operator   string                    // the operator principal id of Local mode
	SubmitRole roles.Role                // the role POST /v1/runs needs (default release: a submit runs a commit's own pipeline config on the coordinator)
	Poll       time.Duration             // events poll interval (default 500 ms)
	MaxLife    time.Duration             // longest events or log stream before the client must reconnect (default 6 h)
	Logf       func(format string, a ...any)
}

// Server is the user API.
type Server struct {
	cfg Config
	lim *limiter
}

// New fills the defaults of cfg.
func New(cfg Config) *Server {
	if cfg.Poll <= 0 {
		cfg.Poll = 500 * time.Millisecond
	}
	if cfg.MaxLife <= 0 {
		cfg.MaxLife = 6 * time.Hour
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.SubmitRole.Rank() == 0 {
		cfg.SubmitRole = roles.Release
	}
	return &Server{cfg: cfg, lim: newLimiter()}
}

type routeSpec struct {
	method, pattern string
	role            roles.Role
	h               http.Handler
}

var routes struct {
	sync.Mutex
	list []routeSpec
}

// RegisterRoute adds a route every Server serves (P7-CACHE-17 calls it for its run-scoped routes). The
// handler runs only after authentication and roles.Authorize(actor, role, ""); it reads the caller with
// ActorFrom and authorizes the project of the run it serves itself. A repeated method and pattern, an
// unknown role or a nil handler is refused.
func RegisterRoute(method, pattern, role string, h http.Handler) error {
	r, ok := roles.ParseRole(role)
	if !ok || h == nil || method == "" || pattern == "" || pattern[0] != '/' {
		return Failf(ReasonBadRequest, "route %s %s needs a role (member, release, admin), a handler and an absolute pattern", method, pattern)
	}
	routes.Lock()
	for _, x := range routes.list {
		if x.method == method && x.pattern == pattern {
			routes.Unlock()
			return Failf(ReasonConflict, "route %s %s is already registered", method, pattern)
		}
	}
	routes.Unlock()
	if err := tryPattern(method, pattern); err != nil {
		return err
	}
	routes.Lock()
	defer routes.Unlock()
	routes.list = append(routes.list, routeSpec{method, pattern, r, h})
	return nil
}

// builtins are the routes every Server serves. The submit route is registered with the member role so the
// wrapper's project-less check passes for a caller scoped to one project; the handler then requires
// Config.SubmitRole for the project in the body.
func (s *Server) builtins() []routeSpec {
	return []routeSpec{
		{http.MethodGet, PathPing, roles.Member, http.HandlerFunc(s.ping)},
		{http.MethodPost, PathRuns, roles.Member, http.HandlerFunc(s.submit)},
		{http.MethodGet, PathRuns, roles.Member, http.HandlerFunc(s.list)},
		{http.MethodGet, PathRunsPfx + "{id}", roles.Member, http.HandlerFunc(s.get)},
		{http.MethodPost, PathRunsPfx + "{id}/cancel", roles.Member, http.HandlerFunc(s.cancel)},
		{http.MethodGet, PathRunsPfx + "{id}/events", roles.Member, http.HandlerFunc(s.events)},
		{http.MethodGet, PathLogsPfx + "{job}", roles.Member, http.HandlerFunc(s.logs)},
		{http.MethodGet, PathEvidence + "{run}", roles.Member, http.HandlerFunc(s.evidence)},
	}
}

// tryPattern reports a pattern the mux would panic on (malformed, or overlapping a built-in or an earlier
// registered route), at registration instead of at serve time.
func tryPattern(method, pattern string) (err error) {
	defer func() {
		if recover() != nil {
			err = Failf(ReasonBadRequest, "route %s %s is not a valid pattern or overlaps another route", method, pattern)
		}
	}()
	mux := http.NewServeMux()
	for _, b := range (&Server{}).builtins() {
		mux.Handle(b.method+" "+b.pattern, http.NotFoundHandler())
	}
	routes.Lock()
	for _, x := range routes.list {
		mux.Handle(x.method+" "+x.pattern, http.NotFoundHandler())
	}
	routes.Unlock()
	mux.Handle(method+" "+pattern, http.NotFoundHandler())
	return nil
}

// Handler serves the built-in routes and every registered one.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, b := range s.builtins() {
		s.add(mux, b)
	}
	routes.Lock()
	extra := append([]routeSpec(nil), routes.list...)
	routes.Unlock()
	sort.Slice(extra, func(i, j int) bool { return extra[i].method+extra[i].pattern < extra[j].method+extra[j].pattern })
	for _, r := range extra {
		s.add(mux, r)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, Failf(ReasonNotFound, "no such route"))
	})
	return mux
}

type actorKey struct{}

// ActorFrom is the authenticated caller of a request served through a Server.
func ActorFrom(ctx context.Context) (roles.Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(roles.Actor)
	return a, ok
}

// add wraps one route: authenticate, then roles.Authorize for the route's role.
func (s *Server) add(mux *http.ServeMux, r routeSpec) {
	mux.Handle(r.method+" "+r.pattern, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		src := sourceOf(req)
		if !s.cfg.Local && !s.lim.allowed(src) {
			s.fail(w, Failf(ReasonRateLimited, "too many failed authentications from this address; wait a minute"))
			return
		}
		actor, err := s.authenticate(req)
		if err != nil {
			s.lim.failed(src)
		} else {
			key := "role:" + actor.ID()
			if !s.lim.allowed(key) {
				s.fail(w, Failf(ReasonRateLimited, "too many role denials for this actor; wait a minute"))
				return
			}
			err = roles.Authorize(req.Context(), actor, r.role, "")
			if roles.ReasonOf(err) != "" {
				s.lim.failed(key)
			}
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		r.h.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), actorKey{}, actor)))
	}))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var statusOf = map[string]int{
	ReasonNoCredential: http.StatusUnauthorized, ReasonBadCertificate: http.StatusUnauthorized, ReasonTokenUnknown: http.StatusUnauthorized,
	ReasonTokenExpired: http.StatusUnauthorized, ReasonTokenRevoked: http.StatusUnauthorized, ReasonPlainHTTP: http.StatusUnauthorized,
	ReasonBadRequest: http.StatusBadRequest, ReasonNotFound: http.StatusNotFound, ReasonConflict: http.StatusConflict,
	ReasonInternal: http.StatusInternalServerError, ReasonRateLimited: http.StatusTooManyRequests,
}

// fail writes the E664 body of err. A role error keeps its reason key; any other error is an internal
// failure whose text stays in the log.
func (s *Server) fail(w http.ResponseWriter, err error) {
	reason, msg := ReasonOf(err), ""
	var ce *Error
	switch {
	case reason != "":
		if asE664(err, &ce) {
			msg = ce.Msg
		}
	case roles.ReasonOf(err) != "":
		reason, msg = roles.ReasonOf(err), "the caller may not do this"
	default:
		s.cfg.Logf("user api: internal error: %v", err)
		reason, msg = ReasonInternal, "internal error"
	}
	status, ok := statusOf[reason]
	if !ok {
		status = http.StatusForbidden
	}
	writeJSON(w, status, Body{Code: CodeUserAPI, Reason: reason, Message: msg})
}
