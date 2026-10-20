package main

// Purpose: the client half of `nself ci nodes <sub> --coordinator <url>` (P7-NODE-26): find the flag in a
// nodes command's argv, build the request from the subcommand's flags and print what the coordinator ran.
// Inputs:  the registered key ("nodes revoke") and the argv after it.
// Outputs: the coordinator's stdout and stderr and its exit code; E664 failures through coordFail.
// Constraints: see nodes_remote.go; nothing is sent when the flags do not parse or the command is local-only.

import (
	"context"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
)

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
