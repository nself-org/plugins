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
	"os"
	"slices"
	"strings"
	"sync"
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

// nodeBusy marks the nodes with an upgrade in flight ("*" is upgrade --all): the agent update engine keeps
// one job per node in a package-level map, and a pull node's engine takes no node lock, so two runs on one
// node would collide. A second request is refused (E664 conflict), not queued. The mark is released by the
// worker goroutine when the handler returns, never by the request, so a client that gave up cannot reopen it.
var nodeBusy = struct {
	sync.Mutex
	m map[string]bool
}{m: map[string]bool{}}

// claimNode marks cmd's node busy for an upgrade (or a rotate in upgrade mode) and returns the release.
func claimNode(cmd clientapi.NodeCommand) (func(), error) {
	if cmd.Verb != "upgrade" && !(cmd.Verb == "rotate" && slices.Contains(cmd.Args, "--mode=upgrade")) {
		return func() {}, nil
	}
	key := cmd.Node
	if key == "" {
		key = "*"
	}
	nodeBusy.Lock()
	defer nodeBusy.Unlock()
	if nodeBusy.m["*"] || (key == "*" && len(nodeBusy.m) > 0) || nodeBusy.m[key] {
		return nil, clientapi.Failf(clientapi.ReasonConflict, "an upgrade is already running on this coordinator for %s", map[bool]string{true: "the fleet", false: "this node"}[key == "*" || nodeBusy.m["*"]])
	}
	nodeBusy.m[key] = true
	return func() { nodeBusy.Lock(); delete(nodeBusy.m, key); nodeBusy.Unlock() }, nil
}

// runNodeCommand is the clientapi.NodeRunner: it runs the local handler of cmd.Verb as actor.
func runNodeCommand(ctx context.Context, actor string, cmd clientapi.NodeCommand) (clientapi.NodeResult, error) {
	st := nodeAdminDeps.Load()
	if st == nil {
		return clientapi.NodeResult{}, clientapi.Failf(clientapi.ReasonNotFound, "node administration is not available on this server")
	}
	release, err := claimNode(cmd)
	if err != nil {
		return clientapi.NodeResult{}, err
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
		defer release()
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
