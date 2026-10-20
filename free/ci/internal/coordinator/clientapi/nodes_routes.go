package clientapi

// Purpose: TEAM node administration over the user API (P7-NODE-26, Epic NODE D14, ADR 0025 §11): the
// /v1/nodes routes behind `nself ci nodes <sub> --coordinator <url>`. list and show need the member role;
// set, drain, enable, revoke, remove, rotate and upgrade need admin. Every route runs the same command
// the local `ci nodes` runs, through a NodeRunner the serve hands in, so the lifecycle code and its audit
// rows (written with the caller's label as the actor) are the same on both paths.
// Inputs:  RegisterNodeRoutes(run); a JSON NodeRequest (mutations) or ?json=1 (reads).
// Outputs: a NodeResult (the command's exit code, stdout and stderr); E664 on refusal.
// Constraints: no add or pair route exists (enrollment is one flow on the coordinator host, Epic D5; the
//          client refuses them with E664 local_only); the route role is admin for a mutation and the
//          handler checks it again, so a route registered with a lower role still fails closed; the
//          caller never supplies argv: each flag is built here from a validated field, always in the
//          --flag=value form, so a value cannot become a second flag; a non-zero exit of the command is
//          HTTP 422 with its NodeResult, an authorization refusal is the usual E664 body; until the serve
//          has started the routes answer not found.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// Paths of the node routes.
const (
	PathNodes        = "/v1/nodes"
	PathNodesPfx     = "/v1/nodes/"
	PathNodesUpgrade = "/v1/nodes/upgrade"
)

// ReasonLocalOnly is the E664 reason of a command that runs only on the coordinator host.
const ReasonLocalOnly = "local_only"

// NodeVerbs are the commands the user API serves, in route order.
var NodeVerbs = []string{"list", "show", "set", "drain", "enable", "revoke", "remove", "rotate", "upgrade"}

// NodeCommand is one validated command for the runner. Args are --flag=value strings built by the route.
type NodeCommand struct {
	Verb, Node string
	Args       []string
	JSON       bool
}

// NodeResult is what the command printed and its exit code.
type NodeResult struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// NodeRunner runs cmd as the caller (actor is the caller's audit label) and captures its output.
type NodeRunner func(ctx context.Context, actor string, cmd NodeCommand) (NodeResult, error)

// NodeRequest is the body of a mutation. Only the fields of the route's verb are read; any other
// non-zero field is refused.
type NodeRequest struct {
	JSON bool `json:"json,omitempty"`
	// set
	ReserveCPU  string `json:"reserve_cpu,omitempty"`
	ReserveMem  string `json:"reserve_mem,omitempty"`
	Interactive string `json:"interactive,omitempty"`
	OnBattery   string `json:"on_battery,omitempty"`
	Persistence string `json:"persistence,omitempty"`
	// rotate
	HostKey string `json:"host_key,omitempty"`
	Mode    string `json:"mode,omitempty"`
	KeyID   string `json:"key_id,omitempty"`
	// upgrade: exactly one of Node and All
	Node           string `json:"node,omitempty"`
	All            bool   `json:"all,omitempty"`
	Version        string `json:"version,omitempty"`
	AllowDowngrade bool   `json:"allow_downgrade,omitempty"`
}

// maxNodeBody caps a mutation body.
const maxNodeBody = 8 << 10

var valueRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/+%=,-]{0,127}$`)

var errServer = New(Config{}) // writes E664 bodies for the registered handlers

// RegisterNodeRoutes registers the /v1/nodes routes. run is read per request and may return nil before
// the serve has started.
func RegisterNodeRoutes(run func() NodeRunner) error {
	n := nodeRoutes{run: run}
	type spec struct {
		method, pattern, verb string
		role                  roles.Role
	}
	var all []spec
	all = append(all, spec{http.MethodGet, PathNodes, "list", roles.Member}, spec{http.MethodGet, PathNodesPfx + "{id}", "show", roles.Member},
		spec{http.MethodPost, PathNodesUpgrade, "upgrade", roles.Admin})
	for _, v := range []string{"set", "drain", "enable", "revoke", "remove", "rotate"} {
		all = append(all, spec{http.MethodPost, PathNodesPfx + "{id}/" + v, v, roles.Admin})
	}
	for _, s := range all {
		if err := RegisterRoute(s.method, s.pattern, string(s.role), n.handler(s.verb, s.role)); err != nil {
			return err
		}
	}
	return nil
}

type nodeRoutes struct{ run func() NodeRunner }

func (n nodeRoutes) handler(verb string, min roles.Role) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFrom(r.Context())
		if !ok {
			errServer.fail(w, Failf(ReasonInternal, "no caller"))
			return
		}
		if err := actor.Can(min, ""); err != nil { // the wrapper authorized already; fail closed if a route was registered lower
			errServer.fail(w, err)
			return
		}
		cmd, err := n.command(verb, r)
		if err != nil {
			errServer.fail(w, err)
			return
		}
		var run NodeRunner
		if n.run != nil {
			run = n.run()
		}
		if run == nil {
			errServer.fail(w, Failf(ReasonNotFound, "node administration is not available on this server"))
			return
		}
		res, err := run(r.Context(), actor.Label(), cmd)
		if err != nil {
			errServer.fail(w, err)
			return
		}
		status := http.StatusOK
		if res.Exit != 0 {
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, res)
	})
}

// command validates the request and builds the runner's command.
func (n nodeRoutes) command(verb string, r *http.Request) (NodeCommand, error) {
	cmd := NodeCommand{Verb: verb}
	var req NodeRequest
	if r.Method == http.MethodPost {
		if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			return cmd, Failf(ReasonBadRequest, "send the body as application/json")
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, maxNodeBody))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			return cmd, Failf(ReasonBadRequest, "the request body is not valid")
		}
	} else {
		req.JSON = r.URL.Query().Get("json") == "1"
	}
	cmd.JSON = req.JSON
	node := r.PathValue("id")
	if verb == "upgrade" {
		node = req.Node
	}
	if node != "" {
		node = strings.ToLower(node)
		if remote.ValidateNodeID(node) != nil {
			return cmd, Failf(ReasonBadRequest, "the node id is not valid")
		}
	}
	cmd.Node = node
	args, err := nodeArgs(verb, req)
	cmd.Args = args
	if err != nil {
		return cmd, err
	}
	if verb == "upgrade" && req.All == (node != "") {
		return cmd, Failf(ReasonBadRequest, "upgrade needs a node or all, not both")
	}
	return cmd, nil
}

// nodeArgs builds the --flag=value list of a verb and refuses a field the verb does not take.
func nodeArgs(verb string, q NodeRequest) ([]string, error) {
	var args []string
	take := func(flag, v string) error {
		if v == "" {
			return nil
		}
		if !valueRe.MatchString(v) {
			return Failf(ReasonBadRequest, "the value of %s is not valid", flag)
		}
		args = append(args, "--"+flag+"="+v)
		return nil
	}
	own := map[string][]string{"set": {"reserve_cpu", "reserve_mem", "interactive", "on_battery", "persistence"},
		"rotate": {"host_key", "mode", "key_id"}, "upgrade": {"node", "all", "version", "allow_downgrade"}}[verb]
	given := map[string]bool{"reserve_cpu": q.ReserveCPU != "", "reserve_mem": q.ReserveMem != "", "interactive": q.Interactive != "",
		"on_battery": q.OnBattery != "", "persistence": q.Persistence != "", "host_key": q.HostKey != "", "mode": q.Mode != "", "key_id": q.KeyID != "",
		"node": q.Node != "", "all": q.All, "version": q.Version != "", "allow_downgrade": q.AllowDowngrade}
	for k, on := range given {
		if on && !slices.Contains(own, k) {
			return nil, Failf(ReasonBadRequest, "%s does not take %s", verb, k)
		}
	}
	for _, f := range []struct{ flag, v string }{{"reserve-cpu", q.ReserveCPU}, {"reserve-mem", q.ReserveMem}, {"interactive", q.Interactive},
		{"on-battery", q.OnBattery}, {"persistence", q.Persistence}, {"host-key", q.HostKey}, {"mode", q.Mode}, {"key-id", q.KeyID}, {"version", q.Version}} {
		if err := take(f.flag, f.v); err != nil {
			return nil, err
		}
	}
	if q.All {
		args = append(args, "--all")
	}
	if q.AllowDowngrade {
		args = append(args, "--allow-downgrade")
	}
	return args, nil
}
