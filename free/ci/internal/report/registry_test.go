package report

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// clock is a settable test clock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// newSender is a Sender on a fresh store with a fixed clock and no jitter
// (Rand 0.5), the github default driven by gh.
func newSender(t *testing.T, gh Reporter) (*Sender, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	return &Sender{Store: openStore(t), Registry: NewRegistry(gh), Now: c.now, Rand: func() float64 { return 0.5 }}, c
}

// recorder is a Reporter that records every status it is asked to post; it
// fails the first failFirst calls.
type recorder struct {
	mu        sync.Mutex
	failFirst int
	calls     int
	posted    []Status
}

func (r *recorder) Post(_ context.Context, s Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.calls <= r.failFirst {
		return context.DeadlineExceeded
	}
	r.posted = append(r.posted, s)
	return nil
}

// TestGoldenUnchanged proves the acceptance line "golden request bodies for
// pass, fail and error runs equal CI-37's goldens (argv of the gh adapter
// unchanged)": the default github route, driven through the sender and the
// exec fake, runs exactly the v1.4 gh argv.
func TestGoldenUnchanged(t *testing.T) {
	golden := map[State][]string{
		Success: {"gh", "api", "--method", "POST", "repos/o/r/statuses/" + sha, "-f", "state=success", "-f", "context=nself-ci", "-f", "description=all green"},
		Failure: {"gh", "api", "--method", "POST", "repos/o/r/statuses/" + sha, "-f", "state=failure", "-f", "context=nself-ci", "-f", "description=lint failed"},
		Error:   {"gh", "api", "--method", "POST", "repos/o/r/statuses/" + sha, "-f", "state=error", "-f", "context=nself-ci", "-f", "description=runner lost"},
	}
	desc := map[State]string{Success: "all green", Failure: "lint failed", Error: "runner lost"}
	for state, want := range golden {
		var got []string
		gh := GH{Capture: func(_ context.Context, argv ...string) (string, int, error) { got = argv; return "", 0, nil }}
		d, _ := newSender(t, gh)
		seq, err := d.Reserve(context.Background(), "o", "r", sha)
		if err != nil {
			t.Fatal(err)
		}
		if err = d.Complete(context.Background(), seq, status(state, desc[state])); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s argv changed:\n got %q\nwant %q", state, got, want)
		}
	}
}

// TestRegistryRouting proves "a fake gitlab reporter registered with a match
// on a gitlab.com remote receives the post and the gh adapter does not".
func TestRegistryRouting(t *testing.T) {
	ctx, gh, gl := context.Background(), &recorder{}, &recorder{}
	d, _ := newSender(t, gh)
	if err := d.Registry.Register("gitlab", func(r string) bool { return strings.Contains(r, "gitlab.com/") }, gl); err != nil {
		t.Fatal(err)
	}
	if err := d.Registry.Register("gitlab", func(string) bool { return true }, gl); err == nil {
		t.Fatal("a kind registers once")
	}
	if err := d.Registry.Register("", nil, nil); err == nil {
		t.Fatal("an empty registration is refused")
	}
	d.Remote = func(s Status) string {
		switch s.Repo {
		case "lab":
			return "https://gitlab.com/o/lab"
		case "odd":
			return "https://example.com/o/odd"
		}
		return "https://github.com/o/" + s.Repo
	}
	for _, repo := range []string{"lab", "r", "odd"} {
		seq, _ := d.Reserve(ctx, "o", repo, sha)
		s := status(Success, "ok")
		s.Repo = repo
		if err := d.Complete(ctx, seq, s); err != nil {
			t.Fatal(err)
		}
	}
	if len(gl.posted) != 1 || gl.posted[0].Repo != "lab" {
		t.Fatalf("gitlab reporter got %+v", gl.posted)
	}
	if len(gh.posted) != 1 || gh.posted[0].Repo != "r" {
		t.Fatalf("the gh adapter got %+v (it must not see the gitlab remote)", gh.posted)
	}
	// A remote nobody accepts is parked with a reason, not lost.
	list, err := d.Store.OutboxList(ctx)
	if err != nil || len(list) != 1 || list[0].State != "failed" || !strings.Contains(list[0].LastError, "no reporter") {
		t.Fatalf("unroutable remote: %+v %v", list, err)
	}
	if _, ok := Resolve("https://github.com/a/b"); !ok {
		t.Fatal("the process-wide registry resolves GitHub remotes")
	}
	if kind, _, ok := d.Registry.Resolve("https://gitlab.com/a/b"); !ok || kind != "gitlab" {
		t.Fatalf("kind = %q %v", kind, ok)
	}
}

// A matcher that panics does not match, and does not take the registry down.
func TestRegistryPanickingMatcher(t *testing.T) {
	r := NewRegistry(&recorder{})
	if err := r.Register("bad", func(string) bool { panic("boom") }, &recorder{}); err != nil {
		t.Fatal(err)
	}
	if kind, _, ok := r.Resolve("https://github.com/a/b"); !ok || kind != KindGitHub {
		t.Fatalf("resolve past a panicking matcher: %q %v", kind, ok)
	}
}
