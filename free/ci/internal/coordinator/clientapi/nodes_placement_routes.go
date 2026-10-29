package clientapi

// Purpose: TEAM node labels and project authorization over the user API (P7-NODE-30, Epic NODE D19): the
// /v1/nodes/{id}/labels and /v1/nodes/{id}/projects routes behind `nself ci nodes label|projects
// --coordinator <url>`. A GET needs the member role (and a grant on all projects, like node list); a POST
// needs admin. Every route runs the same operation the local command runs, through a PlacementRunner the
// serve hands in, so the audit rows are written with the caller's label as the actor on both paths.
// Inputs:  RegisterPlacementRoutes(run); a JSON PlacementRequest (POST) or ?json=1 (GET).
// Outputs: a NodeResult (the command's exit code, stdout and stderr); E664 on refusal.
// Constraints: the route role is admin for a mutation and the handler checks it again, so a route
//          registered with a lower role still fails closed; the caller never supplies argv: labels, keys and
//          project ids are validated here and travel as typed fields; directories, `local` and wildcard
//          grants are the runner's to refuse on a coordinator; until the serve has started the routes answer
//          not found.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// Placement verbs.
const (
	VerbLabel          = "label"
	VerbLabelList      = "label list"
	VerbProjectsAdd    = "projects add"
	VerbProjectsRemove = "projects remove"
	VerbProjectsList   = "projects list"
)

// PlacementCommand is one validated operation for the runner.
type PlacementCommand struct {
	Verb, Node string
	Labels     []string // label: key or key=value
	Remove     []string // label: keys to remove
	Projects   []string // projects add|remove: repo ids
	All        bool
	JSON       bool
}

// PlacementRunner runs cmd as the caller (actor is the caller's audit label) and captures its output.
type PlacementRunner func(ctx context.Context, actor string, cmd PlacementCommand) (NodeResult, error)

// PlacementRequest is the body of a POST. Action is add or remove for the projects route.
type PlacementRequest struct {
	JSON     bool     `json:"json,omitempty"`
	Action   string   `json:"action,omitempty"`
	Labels   []string `json:"labels,omitempty"`
	Remove   []string `json:"remove,omitempty"`
	Projects []string `json:"projects,omitempty"`
	All      bool     `json:"all,omitempty"`
}

const (
	maxPlacementBody  = 16 << 10
	maxPlacementItems = 64
)

var placementItemRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:=-]{0,127}$`)

// RegisterPlacementRoutes registers the labels and projects routes. run is read per request and may
// return nil before the serve has started.
func RegisterPlacementRoutes(run func() PlacementRunner) error {
	p := placementRoutes{run: run}
	for _, s := range []struct {
		method, suffix, verb string
		role                 roles.Role
	}{{http.MethodGet, "labels", VerbLabelList, roles.Member}, {http.MethodPost, "labels", VerbLabel, roles.Admin},
		{http.MethodGet, "projects", VerbProjectsList, roles.Member}, {http.MethodPost, "projects", "", roles.Admin}} {
		if err := RegisterRoute(s.method, PathNodesPfx+"{id}/"+s.suffix, string(s.role), p.handler(s.verb, s.role)); err != nil {
			return err
		}
	}
	return nil
}

type placementRoutes struct{ run func() PlacementRunner }

func (p placementRoutes) handler(verb string, min roles.Role) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := ActorFrom(r.Context())
		if !ok {
			errServer.fail(w, Failf(ReasonInternal, "no caller"))
			return
		}
		if err := actor.Can(min, ""); err != nil {
			errServer.fail(w, err)
			return
		}
		cmd, err := placementCommand(verb, r)
		if err != nil {
			errServer.fail(w, err)
			return
		}
		if r.Method == http.MethodGet && !wholeCoordinator(actor) {
			errServer.fail(w, Failf(ReasonProject, "node information is coordinator-wide: it needs a grant on all projects"))
			return
		}
		var run PlacementRunner
		if p.run != nil {
			run = p.run()
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
		if r.Method == http.MethodGet && res.Exit >= 2 {
			res = NodeResult{Exit: res.Exit, Stderr: "E607: the node registry could not be read\n"}
		}
		status := http.StatusOK
		if res.Exit != 0 {
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, res)
	})
}

// placementCommand validates the request and builds the runner's command.
func placementCommand(verb string, r *http.Request) (PlacementCommand, error) {
	cmd := PlacementCommand{Verb: verb}
	var req PlacementRequest
	if r.Method == http.MethodPost {
		if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			return cmd, Failf(ReasonBadRequest, "send the body as application/json")
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, maxPlacementBody+1))
		if err != nil || len(raw) > maxPlacementBody {
			return cmd, Failf(ReasonBadRequest, "the request body is missing or too long")
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		var extra json.RawMessage
		if dec.Decode(&req) != nil || dec.Decode(&extra) != io.EOF {
			return cmd, Failf(ReasonBadRequest, "the request body is not valid")
		}
	} else {
		req.JSON = r.URL.Query().Get("json") == "1"
	}
	cmd.JSON = req.JSON
	cmd.Node = strings.ToLower(r.PathValue("id"))
	if remote.ValidateNodeID(cmd.Node) != nil {
		return cmd, Failf(ReasonBadRequest, "the node id is not valid")
	}
	if verb == "" { // the projects POST: the action picks the verb
		switch req.Action {
		case "add":
			cmd.Verb = VerbProjectsAdd
		case "remove":
			cmd.Verb = VerbProjectsRemove
		default:
			return cmd, Failf(ReasonBadRequest, "action must be add or remove")
		}
	}
	for _, f := range []struct {
		name string
		own  bool
		list []string
		dst  *[]string
	}{{"labels", cmd.Verb == VerbLabel, req.Labels, &cmd.Labels}, {"remove", cmd.Verb == VerbLabel, req.Remove, &cmd.Remove},
		{"projects", cmd.Verb == VerbProjectsAdd || cmd.Verb == VerbProjectsRemove, req.Projects, &cmd.Projects}} {
		if len(f.list) == 0 {
			continue
		}
		if !f.own {
			return cmd, Failf(ReasonBadRequest, "%s does not take %s", cmd.Verb, f.name)
		}
		if len(f.list) > maxPlacementItems {
			return cmd, Failf(ReasonBadRequest, "too many %s", f.name)
		}
		for _, v := range f.list {
			if !placementItemRe.MatchString(v) {
				return cmd, Failf(ReasonBadRequest, "a value of %s is not valid", f.name)
			}
		}
		*f.dst = f.list
	}
	if req.All && cmd.Verb != VerbProjectsAdd && cmd.Verb != VerbProjectsRemove {
		return cmd, Failf(ReasonBadRequest, "%s does not take all", cmd.Verb)
	}
	cmd.All = req.All
	return cmd, nil
}
