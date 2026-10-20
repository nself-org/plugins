package cloudvm_test

// Purpose: the fake `nself` of the cloud VM tests. The test binary itself is the fake: a symlink named
// `nself` in a temp directory (put on PATH by the rig) re-executes the test binary, and TestMain sees the
// name and runs fakeNselfMain instead of the tests. The child forwards its argv and environment to the
// parent over a unix socket next to the symlink; the parent's bridge answers like `nself infra server`
// (v1 JSON envelopes) on top of internal/burst/fakecloud, and records every call.
// Inputs:  the child's argv and environment. Outputs: stdout, stderr and an exit code.
// Constraints: no network, no fixed port; every token-looking value is synthetic.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/burst/fakecloud"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard) // probing the enrollment listener with bare dials makes net/http log a handshake EOF each
	if filepath.Base(os.Args[0]) == "nself" {
		os.Exit(fakeNselfMain(os.Args[1:]))
	}
	os.Exit(m.Run())
}

type bridgeReq struct {
	Argv, Env []string
}

type bridgeResp struct {
	Stdout, Stderr string
	Exit           int
	// Die makes the child exit without any output (a response lost on the wire).
	Die bool
}

// fakeNselfMain is the child: forward, print, exit.
func fakeNselfMain(args []string) int {
	conn, err := net.Dial("unix", filepath.Join(filepath.Dir(os.Args[0]), "sock"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake nself: no bridge")
		return 99
	}
	defer func() { _ = conn.Close() }()
	if err = json.NewEncoder(conn).Encode(bridgeReq{Argv: args, Env: os.Environ()}); err != nil {
		return 98
	}
	var resp bridgeResp
	if err = json.NewDecoder(conn).Decode(&resp); err != nil || resp.Die {
		return 97
	}
	fmt.Fprint(os.Stderr, resp.Stderr)
	fmt.Fprint(os.Stdout, resp.Stdout)
	return resp.Exit
}

// call is one recorded child invocation.
type call struct {
	Argv     []string
	Env      []string
	UserData []byte
	UDMode   os.FileMode
}

// knobs are the switches of the fake infra CLI; they are read under the bridge lock.
type knobs struct {
	// noUserDataFlag makes `provision --help` omit --user-data-file.
	noUserDataFlag bool
	// ignoreSelector makes list return every server (a CLI that ignored the selector).
	ignoreSelector bool
	// dieOnProvision makes the child die after the server was created (a lost response).
	dieOnProvision bool
	// dropUserData makes provision answer user_data false; stripLabels answers without labels.
	dropUserData, stripLabels bool
	// failCause is the cause of the next failing E744 envelope ("HTTP 429", "network error", ...).
	failCause string
	outage    bool
	// token is the token the fake accepts.
	token string
}

// bridge answers the fake child on top of a fakecloud.
type bridge struct {
	t     *testing.T
	cloud *fakecloud.Cloud
	mu    sync.Mutex
	calls []call
	k     knobs
}

func (b *bridge) set(f func(*knobs)) {
	b.mu.Lock()
	f(&b.k)
	b.mu.Unlock()
}

// newBridge starts the bridge in dir and returns it; the symlink and socket live in dir.
func newBridge(t *testing.T, dir string, cloud *fakecloud.Cloud, token string) *bridge {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(exe, filepath.Join(dir, "nself")); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("bridge socket %s: %v", sock, err)
	}
	b := &bridge{t: t, cloud: cloud, k: knobs{token: token}}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(c)
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return b
}

func (b *bridge) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	var req bridgeReq
	if json.NewDecoder(c).Decode(&req) != nil {
		return
	}
	_ = json.NewEncoder(c).Encode(b.handle(req))
}

func (b *bridge) recorded() []call {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]call(nil), b.calls...)
}

func (b *bridge) callsOf(verb string) []call {
	var out []call
	for _, c := range b.recorded() {
		if len(c.Argv) > 2 && c.Argv[2] == verb {
			out = append(out, c)
		}
	}
	return out
}

func flagValues(argv []string, name string) []string {
	var out []string
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == name {
			out = append(out, argv[i+1])
		}
	}
	return out
}

func envelope(command string, data any) string {
	b, _ := json.MarshalIndent(map[string]any{"schema_version": "1", "command": command, "data": data}, "", "  ")
	return string(b) + "\n"
}

func errEnvelope(command, code, cause string, exit int) bridgeResp {
	b, _ := json.MarshalIndent(map[string]any{"schema_version": "1", "command": command,
		"error": map[string]any{"code": code, "message": "the operation failed", "cause": cause, "exit_code": exit, "class": "infra"}}, "", "  ")
	return bridgeResp{Stdout: string(b) + "\n", Exit: exit}
}

func fcID(n string) string {
	i, _ := strconv.Atoi(n)
	return fmt.Sprintf("fc-%04d", i)
}

func hetznerID(id string) int64 {
	n, _ := strconv.ParseInt(strings.TrimPrefix(id, "fc-"), 10, 64)
	return n
}

func wire(r burst.Resource) map[string]any {
	ip := ""
	if len(r.Addrs) > 0 {
		ip = r.Addrs[0].String()
	}
	return map[string]any{"id": hetznerID(r.ID), "name": r.Name, "status": "running", "created": r.CreatedAt.UTC().Format(time.RFC3339), "ipv4": ip, "labels": r.Labels}
}

func (b *bridge) handle(req bridgeReq) bridgeResp {
	c := call{Argv: req.Argv, Env: req.Env}
	if p := flagValues(req.Argv, "--user-data-file"); len(p) == 1 {
		if st, err := os.Stat(p[0]); err == nil {
			c.UDMode = st.Mode().Perm()
		}
		c.UserData, _ = os.ReadFile(p[0])
	}
	b.mu.Lock()
	b.calls = append(b.calls, c)
	k := b.k
	if b.k.failCause != "" {
		b.k.failCause = ""
	}
	b.mu.Unlock()
	if len(req.Argv) < 3 || req.Argv[0] != "infra" || req.Argv[1] != "server" {
		return errEnvelope("infra", "E401", "", 1)
	}
	verb := req.Argv[2]
	cmd := "infra server " + verb
	if verb == "provision" && len(req.Argv) > 3 && req.Argv[3] == "--help" {
		if k.noUserDataFlag {
			return bridgeResp{Stdout: "Usage: nself infra server provision --name N --type T\n"}
		}
		return bridgeResp{Stdout: "Usage: nself infra server provision --name N --user-data-file <path>\n"}
	}
	if !envHas(req.Env, "NSELF_CI_BURST_TOKEN="+k.token) {
		return errEnvelope(cmd, "E747", "HTTP 401", 3)
	}
	if k.outage || b.cloud == nil {
		return errEnvelope(cmd, "E744", "network error", 4)
	}
	if k.failCause != "" {
		return errEnvelope(cmd, "E744", k.failCause, 4)
	}
	ctx := context.Background()
	switch verb {
	case "provision":
		labels := map[string]string{}
		for _, kv := range flagValues(req.Argv, "--label") {
			k, v, _ := strings.Cut(kv, "=")
			labels[k] = v
		}
		r, err := b.cloud.Provision(ctx, burst.Intent{Token: labels[burst.LabelIntent], Labels: labels})
		if errors.Is(err, fakecloud.ErrLostResponse) || k.dieOnProvision && err == nil {
			return bridgeResp{Die: true}
		}
		if err != nil {
			return errEnvelope(cmd, "E744", "HTTP 422", 4)
		}
		w := wire(r)
		if k.stripLabels {
			delete(w, "labels")
		}
		return bridgeResp{Stderr: "provisioning server\n", Stdout: envelope(cmd, map[string]any{"server": w, "user_data": !k.dropUserData})}
	case "list":
		labels := map[string]string{}
		if !k.ignoreSelector {
			for _, kv := range strings.Split(strings.Join(flagValues(req.Argv, "--label-selector"), ""), ",") {
				if k, v, ok := strings.Cut(kv, "="); ok {
					labels[k] = v
				}
			}
		}
		rs, _ := b.cloud.List(ctx, labels)
		servers := make([]map[string]any, 0, len(rs))
		for _, r := range rs {
			servers = append(servers, wire(r))
		}
		return bridgeResp{Stdout: envelope(cmd, map[string]any{"servers": servers, "count": len(servers)})}
	case "destroy":
		ids := flagValues(req.Argv, "--id")
		if len(ids) != 1 || !b.cloud.Has(fcID(ids[0])) {
			return errEnvelope(cmd, "E745", "HTTP 404", 1)
		}
		_ = b.cloud.Teardown(ctx, fcID(ids[0]))
		n, _ := strconv.ParseInt(ids[0], 10, 64)
		return bridgeResp{Stdout: envelope(cmd, map[string]any{"server_id": n, "snapshot_id": 0, "retained_ips": []any{}, "released_ips": []any{}})}
	}
	return errEnvelope(cmd, "E401", "", 1)
}

func envHas(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}
