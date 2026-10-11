//go:build unix

package main

// Tests for `nself infra server provision --user-data-file`: the exact content
// reaches the client, every refusal happens before any API call, and the
// content is never echoed to stdout, stderr or the std log. Fixtures are
// obviously fake. A failing assertion prints only booleans and lengths, never
// content, so the marker cannot leak through test output.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/nself-infra/internal/server"
)

const udMarker = "NSELFCI-FIXTURE-USERDATA"

const udContent = "#cloud-config\n# " + udMarker + " fake pairing code 000000\n"

// udRecorder records the user data and the number of CreateServer calls.
type udRecorder struct {
	*fakeServerClient
	calls    int
	userData string
}

func (c *udRecorder) CreateServer(ctx context.Context, req server.ProvisionRequest) (*server.Server, error) {
	c.calls++
	c.userData = req.UserData
	return c.fakeServerClient.CreateServer(ctx, req)
}

// failingCreateClient is a udRecorder whose provider call fails.
type failingCreateClient struct{ *udRecorder }

func (c *failingCreateClient) CreateServer(ctx context.Context, req server.ProvisionRequest) (*server.Server, error) {
	c.calls++
	c.userData = req.UserData
	return nil, errors.New("provider says no")
}

func newRecorder() *udRecorder { return &udRecorder{fakeServerClient: newFakeServerClient()} }

// udWrite writes a fixture with the given mode (chmod'd so the umask cannot interfere).
func udWrite(t *testing.T, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func provisionArgs(extra ...string) []string {
	base := []string{"server", "provision", "--name", "ci-1", "--type", "cx22", "--location", "fsn1", "--image", "ubuntu-24.04"}
	return append(base, extra...)
}

// runCaptured runs the CLI with the std log redirected, and reports whether any
// output stream or the log contained the marker.
func runCaptured(t *testing.T, c server.Client, args ...string) (cliResult, bool) {
	t.Helper()
	var logBuf bytes.Buffer
	oldW, oldF := log.Writer(), log.Flags()
	log.SetOutput(&logBuf)
	defer func() { log.SetOutput(oldW); log.SetFlags(oldF) }()
	res := runCLI(t, cliEnv{Client: c}, args...)
	m := []byte(udMarker)
	leaked := bytes.Contains([]byte(res.Stdout), m) || bytes.Contains([]byte(res.Stderr), m) || bytes.Contains(logBuf.Bytes(), m)
	return res, leaked
}

func decodeEnv(t *testing.T, stdout string) envDoc {
	t.Helper()
	var d envDoc
	if err := json.Unmarshal([]byte(stdout), &d); err != nil {
		t.Fatalf("stdout is not a JSON envelope (%d bytes)", len(stdout))
	}
	return d
}

func TestProvisionUserDataNoEcho(t *testing.T) {
	t.Run("human success", func(t *testing.T) {
		rc := newRecorder()
		p := udWrite(t, "ud", []byte(udContent), 0o600)
		res, leaked := runCaptured(t, rc, provisionArgs("--user-data-file", p)...)
		if leaked {
			t.Fatal("marker found in output or log")
		}
		if res.Exit != 0 || rc.calls != 1 || rc.userData != udContent {
			t.Fatalf("exit=%d calls=%d exactMatch=%v", res.Exit, rc.calls, rc.userData == udContent)
		}
	})
	t.Run("json success", func(t *testing.T) {
		rc := newRecorder()
		p := udWrite(t, "ud", []byte(udContent), 0o600)
		res, leaked := runCaptured(t, rc, provisionArgs("--user-data-file", p, "--json")...)
		if leaked {
			t.Fatal("marker found in output or log")
		}
		d := decodeEnv(t, res.Stdout)
		if res.Exit != 0 || d.Error != nil || d.Data["user_data"] != true || rc.userData != udContent || rc.calls != 1 {
			t.Fatalf("exit=%d err=%v user_data=%v calls=%d exactMatch=%v", res.Exit, d.Error != nil, d.Data["user_data"], rc.calls, rc.userData == udContent)
		}
	})
	t.Run("provider failure", func(t *testing.T) {
		for _, js := range []bool{false, true} {
			fc := &failingCreateClient{newRecorder()}
			p := udWrite(t, "ud", []byte(udContent), 0o600)
			args := provisionArgs("--user-data-file", p)
			if js {
				args = append(args, "--json")
			}
			res, leaked := runCaptured(t, fc, args...)
			if leaked {
				t.Fatalf("marker found in output or log (json=%v)", js)
			}
			if res.Exit == 0 || fc.calls != 1 {
				t.Fatalf("json=%v exit=%d calls=%d", js, res.Exit, fc.calls)
			}
		}
	})
	t.Run("refusals", func(t *testing.T) {
		for _, tc := range refusalCases(t) {
			for _, js := range []bool{false, true} {
				rc := newRecorder()
				args := provisionArgs(tc.args...)
				if js {
					args = append(args, "--json")
				}
				res, leaked := runCaptured(t, rc, args...)
				if leaked {
					t.Fatalf("%s: marker found in output or log (json=%v)", tc.name, js)
				}
				if res.Exit == 0 || rc.calls != 0 {
					t.Fatalf("%s: json=%v exit=%d calls=%d", tc.name, js, res.Exit, rc.calls)
				}
			}
		}
	})
}

type refusalCase struct {
	name string
	args []string
	env  bool // asserts the E401 envelope code in --json mode
}

// refusalCases builds the fixtures for every refused input. Files carry the marker
// where the refusal is not about the content, to prove refusals do not echo it.
func refusalCases(t *testing.T) []refusalCase {
	t.Helper()
	dir := t.TempDir()
	good := udWrite(t, "target", []byte(udContent), 0o600)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	notUTF8 := append([]byte(udContent), 0xff, 0xfe)
	flag := "--user-data-file"
	return []refusalCase{
		{"mode 0644", []string{flag, udWrite(t, "m644", []byte(udContent), 0o644)}, true},
		{"mode 0640", []string{flag, udWrite(t, "m640", []byte(udContent), 0o640)}, true},
		{"directory", []string{flag, t.TempDir()}, true},
		{"symlink", []string{flag, link}, true},
		{"too large", []string{flag, udWrite(t, "big", bytes.Repeat([]byte("a"), maxUserDataBytes+1), 0o600)}, true},
		{"empty", []string{flag, udWrite(t, "empty", nil, 0o600)}, true},
		{"invalid utf-8", []string{flag, udWrite(t, "bin", notUTF8, 0o600)}, false},
		{"missing", []string{flag, filepath.Join(dir, "absent")}, false},
		{"empty path", []string{flag, ""}, false},
	}
}

func TestProvisionUserDataRefusals(t *testing.T) {
	for _, tc := range refusalCases(t) {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rc := newRecorder()
			res := runCLI(t, cliEnv{Client: rc}, provisionArgs(tc.args...)...)
			if res.Exit == 0 || rc.calls != 0 {
				t.Fatalf("human: exit=%d calls=%d", res.Exit, rc.calls)
			}
			rc = newRecorder()
			res = runCLI(t, cliEnv{Client: rc}, provisionArgs(append(tc.args, "--json")...)...)
			d := decodeEnv(t, res.Stdout)
			if res.Exit == 0 || rc.calls != 0 || d.Error == nil {
				t.Fatalf("json: exit=%d calls=%d hasError=%v", res.Exit, rc.calls, d.Error != nil)
			}
			if d.Error.Code != "E401" {
				t.Fatalf("json: error code = %q, want E401", d.Error.Code)
			}
			if strings.Contains(d.Error.Message, "/") {
				t.Fatal("envelope message carries a path")
			}
		})
	}
	t.Run("another owner", func(t *testing.T) {
		old := userDataUID
		userDataUID = func() int { return os.Getuid() + 1 }
		defer func() { userDataUID = old }()
		p := udWrite(t, "ud", []byte(udContent), 0o600)
		rc := newRecorder()
		res := runCLI(t, cliEnv{Client: rc}, provisionArgs("--user-data-file", p, "--json")...)
		d := decodeEnv(t, res.Stdout)
		if res.Exit == 0 || rc.calls != 0 || d.Error == nil || d.Error.Code != "E401" {
			t.Fatalf("exit=%d calls=%d hasError=%v", res.Exit, rc.calls, d.Error != nil)
		}
	})
	t.Run("exactly 32768 bytes accepted", func(t *testing.T) {
		data := bytes.Repeat([]byte("a"), maxUserDataBytes)
		p := udWrite(t, "max", data, 0o600)
		rc := newRecorder()
		res := runCLI(t, cliEnv{Client: rc}, provisionArgs("--user-data-file", p)...)
		if res.Exit != 0 || rc.calls != 1 || len(rc.userData) != maxUserDataBytes {
			t.Fatalf("exit=%d calls=%d len=%d", res.Exit, rc.calls, len(rc.userData))
		}
	})
}

func TestProvisionWithoutUserDataUnchanged(t *testing.T) {
	rc := newRecorder()
	res := runCLI(t, cliEnv{Client: rc}, provisionArgs("--json")...)
	d := decodeEnv(t, res.Stdout)
	if res.Exit != 0 || d.Error != nil || rc.calls != 1 || rc.userData != "" {
		t.Fatalf("exit=%d calls=%d userDataLen=%d", res.Exit, rc.calls, len(rc.userData))
	}
	if v, ok := d.Data["user_data"]; !ok || v != false {
		t.Fatalf("data.user_data = %v (present=%v), want false", v, ok)
	}
}
