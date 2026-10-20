package main

// Purpose: tests of `nself-ci-agent pull` that need no coordinator: usage, the not-joined refusal, the coordinator
//          id, the clock skew marker and the drain watcher.
// Inputs:  a temp agent state dir. Constraints: no network.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func TestPullUsage(t *testing.T) {
	for _, args := range [][]string{{"pull", "extra"}, {"pull", "--slots", "-1"}, {"pull", "--bogus"}, {"pull", "--coordinator", "bad id!"}} {
		code, _, errOut := runCmd(t, args...)
		if code != 2 || !strings.Contains(errOut, "usage: nself-ci-agent pull") && !strings.Contains(errOut, "flag provided") {
			t.Fatalf("%v: code %d %q", args, code, errOut)
		}
	}
}

func TestPullFlagsParse(t *testing.T) {
	var errb bytes.Buffer
	f, ok := parsePull([]string{"--slots", "3", "--coordinator", "c-1", "--project", "/p", "--secrets-eligible", "--dedicated"}, IO{Stderr: &errb})
	if !ok || f.slots != 3 || f.coordinator != "c-1" || f.project != "/p" || !f.secretsOK || !f.dedicated {
		t.Fatalf("flags = %+v ok=%v %s", f, ok, errb.String())
	}
	if f, ok = parsePull(nil, IO{Stderr: &errb}); !ok || f.slots != 0 || f.coordinator != "" || f.secretsOK {
		t.Fatalf("defaults = %+v", f)
	}
}

func TestPullRefusesANodeThatNeverJoined(t *testing.T) {
	useState(t)
	code, _, errOut := runCmd(t, "pull")
	if code != 1 || !strings.Contains(errOut, "has not joined") {
		t.Fatalf("code %d %q", code, errOut)
	}
}

func TestPullRefusesAnEnrollmentWithoutAnHTTPSURL(t *testing.T) {
	state := useState(t)
	writeEnrollment(t, state, "http://127.0.0.1:1")
	code, _, errOut := runCmd(t, "pull")
	if code != 1 || !strings.Contains(errOut, "https agent URL") {
		t.Fatalf("code %d %q", code, errOut)
	}
}

func TestCoordinatorIDFromTheRootFingerprint(t *testing.T) {
	for in, want := range map[string]string{
		"SHA256:ABCdef0123456789+/xyz": "c-abcdef0123456789",
		"SHA256:ab":                    "c-ab",
		"":                             "c-",
		"SHA256:0123456789abcdef0123":  "c-0123456789abcdef",
	} {
		got := coordinatorID(in)
		if got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
		if !idRE.MatchString(got) && got != "c-" {
			t.Errorf("%q -> %q is not a valid coordinator id", in, got)
		}
	}
}

func TestNoteSkewWritesAndClearsTheMarker(t *testing.T) {
	state := t.TempDir()
	marker := filepath.Join(state, "clock_skew")
	noteSkew(state, 3*time.Minute)
	if b, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(b)) != "3m0s" {
		t.Fatalf("marker = %q %v", b, err)
	}
	if st, _ := os.Stat(marker); st.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode %v", st.Mode().Perm())
	}
	noteSkew(state, -5*time.Minute)
	if b, _ := os.ReadFile(marker); strings.TrimSpace(string(b)) != "-5m0s" {
		t.Fatalf("negative marker = %q", b)
	}
	noteSkew(state, 119*time.Second)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("an in-tolerance clock left the marker")
	}
	noteSkew(state, -120*time.Second)
	noteSkew(state, 120*time.Second)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("exactly the tolerance must be accepted")
	}
}

type oneShot struct {
	msgs []protocol.Message
}

func (o *oneShot) Send(protocol.Message) error { return nil }
func (o *oneShot) Close() error                { return nil }
func (o *oneShot) Recv() (protocol.Message, error) {
	m := o.msgs[0]
	o.msgs = o.msgs[1:]
	return m, nil
}

func TestDrainWatchRemembersADrain(t *testing.T) {
	w := &drainWatch{Frames: &oneShot{msgs: []protocol.Message{{Type: "heartbeat", Body: &protocol.Heartbeat{}}, {Type: "drain", Body: &protocol.Drain{Deadline: "x"}}}}}
	if _, err := w.Recv(); err != nil || w.drained() {
		t.Fatal("a heartbeat is not a drain")
	}
	if _, err := w.Recv(); err != nil || !w.drained() {
		t.Fatal("the drain was not remembered")
	}
}
