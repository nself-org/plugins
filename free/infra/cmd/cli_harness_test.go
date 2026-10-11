// Purpose: run the infra command tree in-process the way main does, with the
// provider and terraform sides mocked, and capture stdout, stderr and the exit
// status.
//
// Inputs: a cliEnv (stub tofu mode, TTY answer, fake server client) and args.
//
// Outputs: cliResult{Stdout, Stderr, Exit}.
//
// Constraints: no network, no cloud credentials, no spend. terraform is a shell
// stub named tofu on a private PATH; it appends its arguments to STUB_LOG, so a
// test can prove terraform was (or was never) invoked. The only token anywhere
// is the obviously fake one below.
package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/nself-org/nself-infra/internal/server"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var rootOnce sync.Once

const fakeToken = "fake-token-for-tests"

// stubScript is the tofu stand-in. It prints on stdout (to prove stdout
// isolation), records its arguments, and fails when STUB_MODE is fail.
const stubScript = `#!/bin/sh
echo "stub-tofu stdout line"
echo "$*" >> "$STUB_LOG"
if [ "$STUB_MODE" = fail ]; then
  echo "stub-tofu failure text" >&2
  exit 1
fi
exit 0
`

type cliEnv struct {
	Tofu       string        // "ok", "fail" or "" (no tofu or terraform on PATH)
	TTY        bool          // answer of stdinIsTTY
	Client     server.Client // fake client for the server commands; nil keeps the real factory
	NoTok      bool          // clear every token variable (real factory then has none)
	RealTTY    bool          // keep the production stdinIsTTY (the test sets os.Stdin)
	realClient bool          // keep the real client factory (buildServerClient)
}

type cliResult struct {
	Stdout, Stderr string
	Exit           int
	TofuLog        string // arguments of every stub call, "" when never called
}

// runCLI executes args as `nself infra <args>` and restores every global it touched.
func runCLI(t *testing.T, env cliEnv, args ...string) cliResult {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if env.Tofu != "" {
		if err := os.WriteFile(filepath.Join(bin, "tofu"), []byte(stubScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(dir, "tofu.log")
	t.Setenv("PATH", bin)
	t.Setenv("STUB_LOG", log)
	t.Setenv("STUB_MODE", env.Tofu)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("HCLOUD_TOKEN", "")
	t.Setenv(TerraformTokenEnvVar, "")
	t.Setenv("HETZNER_NSELF_TOKEN", fakeToken)
	if env.NoTok {
		t.Setenv("HETZNER_NSELF_TOKEN", "")
	}

	oldTTY, oldClient := stdinIsTTY, newServerClient
	if !env.RealTTY {
		stdinIsTTY = func() bool { return env.TTY }
	}
	if env.Client != nil {
		newServerClient = func(*cobra.Command) (server.Client, error) { return env.Client, nil }
	}
	defer func() { stdinIsTTY, newServerClient = oldTTY, oldClient }()

	outF, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	errF, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outF, errF
	rootOnce.Do(configureRoot)
	resetAllFlags(infraCmd)
	ran = false
	js := wantsJSON(args)
	infraCmd.SilenceErrors = js
	infraCmd.SilenceUsage = true
	infraCmd.SetArgs(args)
	exit := run(args, js, os.Stdout, os.Stderr) // the same entry point main uses
	os.Stdout, os.Stderr = oldOut, oldErr
	_ = outF.Close()
	_ = errF.Close()

	res := cliResult{Exit: exit}
	if b, err := os.ReadFile(outF.Name()); err == nil {
		res.Stdout = string(b)
	}
	if b, err := os.ReadFile(errF.Name()); err == nil {
		res.Stderr = string(b)
	}
	if b, err := os.ReadFile(log); err == nil {
		res.TofuLog = string(b)
	}
	return res
}

// resetAllFlags puts every flag of the tree back to its default.
func resetAllFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if r, ok := f.Value.(sliceResetter); ok {
			_ = r.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
	for _, sub := range c.Commands() {
		resetAllFlags(sub)
	}
}

// errClient wraps the in-memory fake and fails the named operations.
type errClient struct {
	*fakeServerClient
	get, list, create, change, del, ipSet error
}

func (e *errClient) GetServer(c context.Context, id int64) (*server.Server, error) {
	if e.get != nil {
		return nil, e.get
	}
	return e.fakeServerClient.GetServer(c, id)
}

func (e *errClient) ListServers(c context.Context, o server.ListOptions) ([]server.Server, error) {
	if e.list != nil {
		return nil, e.list
	}
	return e.fakeServerClient.ListServers(c, o)
}

func (e *errClient) CreateServer(c context.Context, r server.ProvisionRequest) (*server.Server, error) {
	if e.create != nil {
		return nil, e.create
	}
	return e.fakeServerClient.CreateServer(c, r)
}

func (e *errClient) ChangeServerType(c context.Context, id int64, t string, up bool) (*server.Action, error) {
	if e.change != nil {
		return nil, e.change
	}
	return e.fakeServerClient.ChangeServerType(c, id, t, up)
}

func (e *errClient) DeleteServer(c context.Context, id int64) error {
	if e.del != nil {
		return e.del
	}
	return e.fakeServerClient.DeleteServer(c, id)
}

func (e *errClient) SetPrimaryIPAutoDelete(c context.Context, id int64, v bool) error {
	if e.ipSet != nil {
		return e.ipSet
	}
	return e.fakeServerClient.SetPrimaryIPAutoDelete(c, id, v)
}
