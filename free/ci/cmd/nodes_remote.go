package main

// Purpose: TEAM node administration over the user API (P7-NODE-26, Epic NODE D14): the client half
// (`nself ci nodes <sub> --coordinator <url>`) and the coordinator half (the runner behind the /v1/nodes
// routes of internal/coordinator/clientapi, registered with the serve).
// Inputs:  argv of a `nodes` subcommand; the coordinator's store and data dir while a serve runs.
// Outputs: client: the coordinator's stdout and stderr and its exit code; server: a clientapi.NodeResult.
// Constraints: V15 only; the flag selects the remote path, so without it every `ci nodes` command runs
//          locally with no role check (access to the data dir is admin access, Epic D1); NSELF_CI_COORDINATOR
//          and the user config do not redirect node commands; `nodes add` and `nodes pair` (enrollment is
//          one flow on the coordinator host, Epic D5) and the other commands the API does not serve are
//          refused with E664 local_only before anything is sent; the server runs the same handlers the
//          local commands run, with the caller's audit label as the actor, a store it does not close and
//          no stdin, so a prompt can never wait on a remote caller.

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// nodesRemoteSubs are the `nodes` commands that take --coordinator: the routed ones and the ones refused.
var nodesRemoteSubs = map[string]bool{"list": true, "show": true, "set": true, "drain": true, "enable": true, "revoke": true,
	"remove": true, "rotate": true, "upgrade": true, "add": true, "pair": true, "test": true}

// nodesRoutedSubs are the commands with a /v1/nodes route.
var nodesRoutedSubs = map[string]bool{"list": true, "show": true, "set": true, "drain": true, "enable": true, "revoke": true,
	"remove": true, "rotate": true, "upgrade": true}

const (
	nodesRunTimeout = 2 * time.Hour // an upgrade waits for running jobs, node by node
	nodesOutCap     = 1 << 20
)

// nodeAdminState is what the routes need from the running serve.
type nodeAdminState struct {
	store *store.Store
	home  string
}

var nodeAdminDeps atomic.Pointer[nodeAdminState]

func init() {
	// Add --coordinator to the flag sets the lifecycle owns (help and the generated reference list it).
	for _, sub := range []string{"add", "list", "show", "test", "drain", "enable", "revoke", "remove", "rotate"} {
		lifecycle.RegisterFlags(sub, func(fs *flag.FlagSet) {
			fs.String("coordinator", "", "run this on the TEAM coordinator at https://host[:port] (an admin or member user credential; not for add, pair or test)")
		})
	}
	if err := clientapi.RegisterNodeRoutes(func() clientapi.NodeRunner {
		if nodeAdminDeps.Load() == nil {
			return nil
		}
		return runNodeCommand
	}); err != nil {
		panic("nself-ci: " + err.Error())
	}
	if err := coordinator.RegisterListener("node-admin-routes", "node-admin-routes", "on", startNodeAdmin); err != nil {
		panic("nself-ci: " + err.Error())
	}
}

// startNodeAdmin hands the coordinator's store to the runner. It binds nothing.
func startNodeAdmin(ctx context.Context, addr string, c *coordinator.Coordinator) (func(context.Context) error, error) {
	if ctx.Err() != nil || addr == "off" {
		return noop, nil
	}
	if c.Store() == nil {
		c.Logf("node administration routes are off: the coordinator has no store")
		return noop, nil
	}
	nodeAdminDeps.Store(&nodeAdminState{store: c.Store(), home: serveHome()})
	return func(context.Context) error { nodeAdminDeps.Store(nil); return nil }, nil
}

// capWriter keeps at most nodesOutCap bytes and drops the rest.
type capWriter struct {
	b    bytes.Buffer
	over bool
}

func (w *capWriter) Write(p []byte) (int, error) {
	room := nodesOutCap - w.b.Len()
	if room > 0 {
		w.b.Write(p[:min(len(p), room)])
	}
	if len(p) > room {
		w.over = true
	}
	return len(p), nil
}

// nodeSerial lets one upgrade or rotate run at a time in this process: the agent update engine keeps one
// job per node in a package-level map, so two concurrent requests for a node would share it.
var nodeSerial = make(chan struct{}, 1)

// runNodeCommand is the clientapi.NodeRunner: it runs the local handler of cmd.Verb as actor.
func runNodeCommand(ctx context.Context, actor string, cmd clientapi.NodeCommand) (clientapi.NodeResult, error) {
	st := nodeAdminDeps.Load()
	if st == nil {
		return clientapi.NodeResult{}, clientapi.Failf(clientapi.ReasonNotFound, "node administration is not available on this server")
	}
	if cmd.Verb == "upgrade" || cmd.Verb == "rotate" {
		select {
		case nodeSerial <- struct{}{}:
			defer func() { <-nodeSerial }()
		case <-ctx.Done():
			return clientapi.NodeResult{}, ctx.Err()
		}
	}
	var out, errw capWriter
	env := lifecycle.Env{Out: &out, Err: &errw,
		Open: func(_ context.Context, _ string) (*lifecycle.Deps, func(), error) {
			d := lifecycle.NewDeps(st.store, st.home, "", actor)
			d.Stdin, d.TTY = strings.NewReader(""), false
			return d, func() {}, nil
		}}
	args := cmd.Args
	if cmd.Node != "" {
		args = append([]string{cmd.Node}, args...)
	}
	if cmd.JSON {
		args = append(args, "--json")
	}
	done := make(chan int, 1)
	go func() {
		switch cmd.Verb {
		case "set":
			done <- runNodesSet(args, env, &out, &errw)
		case "upgrade":
			done <- update.Run(args, update.Env{Lifecycle: env, DefaultVersion: agentPinnedVersion,
				BaseURL: os.Getenv("NSELF_CI_AGENT_RELEASE_URL"), Verifier: agentVerifier}, &out, &errw)
		default:
			done <- lifecycle.Run(cmd.Verb, args, env)
		}
	}()
	select {
	case code := <-done:
		if out.over || errw.over {
			return clientapi.NodeResult{Exit: max(code, 2), Stderr: "the answer is longer than the 1 MiB limit of the user API; read it on the coordinator host\n"}, nil
		}
		return clientapi.NodeResult{Exit: code, Stdout: out.b.String(), Stderr: errw.b.String()}, nil
	case <-time.After(nodesRunTimeout):
		return clientapi.NodeResult{}, errors.New("node command timed out")
	case <-ctx.Done():
		return clientapi.NodeResult{}, ctx.Err()
	}
}

// nodesRemoteDispatch is the client half. handled is false unless argv names a nodes command with
// --coordinator; the caller then runs the local handler.
func nodesRemoteDispatch(key string, args []string) (code int, handled bool) {
	sub, ok := strings.CutPrefix(key, "nodes ")
	if !ok || !nodesRemoteSubs[sub] {
		return 0, false
	}
	url, rest, found, err := takeCoordinatorFlag(args)
	if !found && err == nil {
		return 0, false
	}
	out, errw := io.Writer(os.Stdout), io.Writer(os.Stderr)
	if coordOut != nil {
		out = coordOut
	}
	if coordErr != nil {
		errw = coordErr
	}
	asJSON := hasJSONArg(rest)
	if err == nil && !nodesRoutedSubs[sub] {
		err = clientapi.Failf(clientapi.ReasonLocalOnly, "nodes %s runs only on the coordinator host: enroll nodes there with `nself ci nodes add` and `nself ci nodes pair` (hq NODE D5), then administer them from here", sub)
	}
	var req clientapi.NodeRequest
	var id string
	if err == nil {
		id, req, err = nodesRemoteRequest(sub, rest)
	}
	if err != nil {
		return coordFail(out, errw, asJSON, err), true
	}
	coordinatorFlag = url
	defer func() { coordinatorFlag = "" }()
	cl, _, _, err := coordResolver.Client()
	if err != nil {
		return coordFail(out, errw, req.JSON, err), true
	}
	ctx, cancel := context.WithTimeout(context.Background(), nodesRunTimeout)
	defer cancel()
	var res clientapi.NodeResult
	switch sub {
	case "list":
		res, err = cl.NodeList(ctx, req.JSON)
	case "show":
		res, err = cl.NodeShow(ctx, id, req.JSON)
	default:
		res, err = cl.NodeDo(ctx, sub, id, req)
	}
	if err != nil {
		return coordFail(out, errw, req.JSON, err), true
	}
	_, _ = io.WriteString(out, res.Stdout)
	_, _ = io.WriteString(errw, res.Stderr)
	return res.Exit, true
}

// takeCoordinatorFlag removes --coordinator from args.
func takeCoordinatorFlag(args []string) (url string, rest []string, found bool, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "coordinator" {
			rest = append(rest, a)
			continue
		}
		found = true
		if !hasVal {
			if i+1 >= len(args) {
				return "", nil, true, clientapi.Failf(clientapi.ReasonBadRequest, "--coordinator needs a URL")
			}
			i++
			val = args[i]
		}
		url = val
	}
	if found && url == "" {
		err = clientapi.Failf(clientapi.ReasonBadRequest, "--coordinator needs a URL")
	}
	return url, rest, found, err
}

func hasJSONArg(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" || a == "--json=true" || a == "-json=true" {
			return true
		}
	}
	return false
}

// nodesRemoteRequest parses the flags of a routed command into the request body.
func nodesRemoteRequest(sub string, args []string) (id string, q clientapi.NodeRequest, err error) {
	fs := flag.NewFlagSet("nodes "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&q.JSON, "json", false, "")
	switch sub {
	case "set":
		fs.StringVar(&q.ReserveCPU, "reserve-cpu", "", "")
		fs.StringVar(&q.ReserveMem, "reserve-mem", "", "")
		fs.StringVar(&q.Interactive, "interactive", "", "")
		fs.StringVar(&q.OnBattery, "on-battery", "", "")
		fs.StringVar(&q.Persistence, "persistence", "", "")
	case "rotate":
		fs.StringVar(&q.HostKey, "host-key", "", "")
		fs.StringVar(&q.Mode, "mode", "", "")
		fs.StringVar(&q.KeyID, "key-id", "", "")
		fs.BoolFunc("cert", "", func(string) error { q.Mode = "cert"; return nil })
		fs.BoolFunc("node-key", "", func(string) error { q.Mode = "node-key"; return nil })
	case "upgrade":
		fs.StringVar(&q.Version, "version", "", "")
		fs.BoolVar(&q.All, "all", false, "")
		fs.BoolVar(&q.AllowDowngrade, "allow-downgrade", false, "")
	}
	var pos []string
	for {
		if perr := fs.Parse(args); perr != nil {
			return "", q, clientapi.Failf(clientapi.ReasonBadRequest, "%s", strings.Join(strings.Fields(perr.Error()), " "))
		}
		if fs.NArg() == 0 {
			break
		}
		pos, args = append(pos, fs.Arg(0)), fs.Args()[1:]
	}
	want := 1
	if sub == "list" || (sub == "upgrade" && q.All) {
		want = 0
	}
	if len(pos) != want {
		return "", q, clientapi.Failf(clientapi.ReasonBadRequest, "usage: nself ci nodes %s%s --coordinator <url>", sub, map[bool]string{true: "", false: " <node>"}[want == 0])
	}
	if want == 1 {
		id = strings.ToLower(pos[0])
		if sub == "upgrade" {
			q.Node, id = id, ""
		}
	}
	return id, q, nil
}
