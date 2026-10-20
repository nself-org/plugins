package report

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

type fake struct {
	posted []Status
	fail   bool
}

func (f *fake) Post(_ context.Context, s Status) error {
	if f.fail {
		return errors.New("offline")
	}
	f.posted = append(f.posted, s)
	return nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func status(state State, desc string) Status {
	return Status{Owner: "o", Repo: "r", Revision: sha, State: state, Description: desc}
}

func TestOutboxReverseSeq(t *testing.T) {
	ctx, f := context.Background(), &fake{}
	o := &Outbox{Store: openStore(t), Reporter: f, Owner: "o", Repo: "r"}
	older, _ := o.Reserve(ctx, sha)
	newer, _ := o.Reserve(ctx, sha)
	if ok, err := o.Complete(ctx, newer, status(Success, "new")); err != nil || !ok {
		t.Fatalf("newest: %v %v", ok, err)
	}
	if ok, err := o.Complete(ctx, older, status(Failure, "old")); err != nil || ok {
		t.Fatalf("an older pipeline finishing late must not post: %v %v", ok, err)
	}
	if len(f.posted) != 1 || f.posted[0].Description != "new" {
		t.Fatalf("final state is not the newest pipeline's: %+v", f.posted)
	}
}

func TestOutboxDrainAfterFailure(t *testing.T) {
	ctx, f := context.Background(), &fake{fail: true}
	o := &Outbox{Store: openStore(t), Reporter: f, Owner: "o", Repo: "r"}
	seq, _ := o.Reserve(ctx, sha)
	if ok, err := o.Complete(ctx, seq, status(Success, "ok")); ok || err == nil {
		t.Fatalf("a failed delivery must report the error: %v %v", ok, err)
	}
	f.fail = false
	if n, err := o.Drain(ctx); err != nil || n != 1 {
		t.Fatalf("drain: %d %v", n, err)
	}
	if n, _ := o.Drain(ctx); n != 0 || len(f.posted) != 1 {
		t.Fatalf("a posted entry must not post twice: %d %+v", n, f.posted)
	}
}

func TestOutboxWithoutReporterKeepsEntry(t *testing.T) {
	ctx := context.Background()
	st := openStore(t)
	o := &Outbox{Store: st, Owner: "o", Repo: "r"}
	seq, _ := o.Reserve(ctx, sha)
	if ok, err := o.Complete(ctx, seq, status(Success, "ok")); ok || err != nil {
		t.Fatalf("no reporter: %v %v", ok, err)
	}
	left, _ := st.OutboxUnposted(ctx)
	if len(left) != 1 {
		t.Fatalf("entry must wait for a reporter, got %d", len(left))
	}
}

func TestStatusValidate(t *testing.T) {
	bad := []Status{{Owner: "o/x", Repo: "r", Revision: sha, State: Success}, {Owner: "o", Repo: "r", Revision: "HEAD", State: Success}, {Owner: "o", Repo: "r", Revision: sha, State: "good"}}
	for _, s := range bad {
		if s.Validate() == nil {
			t.Fatalf("%+v must be refused", s)
		}
	}
}

func TestGHArgs(t *testing.T) {
	args := strings.Join(Args(status(Success, strings.Repeat("x", 200))), " ")
	if !strings.Contains(args, "context=nself-ci") || strings.Contains(args, "target_url") {
		t.Fatalf("argv: %s", args)
	}
	if !strings.Contains(args, "repos/o/r/statuses/"+sha) {
		t.Fatalf("endpoint: %s", args)
	}
	if got := Truncate(strings.Repeat("é", 200)); len([]rune(got)) != MaxDescription {
		t.Fatalf("description runes = %d", len([]rune(got)))
	}
}

func TestParseRemote(t *testing.T) {
	for in, want := range map[string]string{"git@github.com:nself-org/cli.git": "nself-org/cli", "https://github.com/a/b": "a/b", "ssh://git@github.com/a/b.git": "a/b", "https://gitlab.com/a/b": "", "": "", "https://github.com/a/b/c": ""} {
		o, r, ok := ParseRemote(in)
		if got := o + "/" + r; ok && got != want || !ok && want != "" {
			t.Fatalf("%q -> %q %v", in, got, ok)
		}
	}
}

func TestResolveReasons(t *testing.T) {
	cap := func(remote string, remoteExit, authExit int) func(context.Context, ...string) (string, int, error) {
		return func(_ context.Context, argv ...string) (string, int, error) {
			if argv[0] == "git" {
				return remote, remoteExit, nil
			}
			return "", authExit, nil
		}
	}
	ctx := context.Background()
	if _, _, _, why := (GH{Capture: cap("", 2, 0)}).Resolve(ctx, "."); why != ReasonNoRemote {
		t.Fatalf("no remote: %q", why)
	}
	if _, _, _, why := (GH{Capture: cap("https://github.com/a/b", 0, 1)}).Resolve(ctx, "."); why != ReasonNoAuth {
		t.Fatalf("no auth: %q", why)
	}
	r, o, n, why := (GH{Capture: cap("git@github.com:a/b.git\n", 0, 0)}).Resolve(ctx, ".")
	if r == nil || o != "a" || n != "b" || why != "" {
		t.Fatalf("resolve: %v %s %s %q", r, o, n, why)
	}
}

func TestGHPostRunsGH(t *testing.T) {
	var got []string
	g := GH{Capture: func(_ context.Context, argv ...string) (string, int, error) { got = argv; return "", 0, nil }}
	if err := g.Post(context.Background(), status(Failure, "x")); err != nil || got[0] != "gh" {
		t.Fatalf("%v %v", got, err)
	}
	g.Capture = func(context.Context, ...string) (string, int, error) { return "HTTP 404", 1, nil }
	if err := g.Post(context.Background(), status(Failure, "x")); err == nil {
		t.Fatal("non-zero gh exit must be an error")
	}
}

// A fork and its upstream share commits; their statuses must not order or
// supersede each other in a shared state directory.
func TestOutboxIsPerRepository(t *testing.T) {
	ctx, f, st := context.Background(), &fake{}, openStore(t)
	up := &Outbox{Store: st, Reporter: f, Owner: "o", Repo: "r"}
	fork := &Outbox{Store: st, Reporter: f, Owner: "me", Repo: "r"}
	a, _ := up.Reserve(ctx, sha)
	b, _ := fork.Reserve(ctx, sha)
	if a != 1 || b != 1 {
		t.Fatalf("each repository counts its own seq: %d %d", a, b)
	}
	forkStatus := status(Failure, "fork")
	forkStatus.Owner = "me"
	if ok, err := fork.Complete(ctx, b, forkStatus); err != nil || !ok {
		t.Fatalf("fork: %v %v", ok, err)
	}
	if ok, err := up.Complete(ctx, a, status(Success, "upstream")); err != nil || !ok {
		t.Fatalf("upstream must still post: %v %v", ok, err)
	}
}

// A poster suspended past the claim TTL lands its older status after the
// newer poster's: the outbox delivers the newest status again, so the host
// ends on it.
func TestOutboxHealsALateOlderPost(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	f := &takeover{st: st, advance: func() { now = now.Add(2 * time.Minute) }}
	o := &Outbox{Store: st, Reporter: f, Owner: "o", Repo: "r"}
	a, _ := o.Reserve(ctx, sha)
	b, _ := o.Reserve(ctx, sha)
	newer := status(Failure, "newer")
	body, _ := json.Marshal(newer)
	if err = st.OutboxReady(ctx, sha, key("o", "r"), b, body); err != nil {
		t.Fatal(err)
	}
	f.seq = b
	if _, err = o.Complete(ctx, a, status(Success, "older")); err != nil {
		t.Fatal(err)
	}
	last := f.posted[len(f.posted)-1]
	if last.Description != "newer" || len(f.posted) < 3 {
		t.Fatalf("the host must end on the newest status: %+v", f.posted)
	}
}

// takeover is a Reporter whose first post is the suspended one: while it is
// "in flight" the claim expires and the newer poster claims, posts and
// acknowledges.
type takeover struct {
	fake
	st      *store.Store
	seq     int64
	advance func()
	done    bool
}

func (t *takeover) Post(ctx context.Context, s Status) error {
	if !t.done {
		t.done = true
		t.advance()
		_, tok, _ := t.st.OutboxClaim(ctx, sha, key("o", "r"), t.seq)
		t.posted = append(t.posted, status(Failure, "newer"))
		_ = t.st.OutboxPosted(ctx, sha, key("o", "r"), t.seq, tok)
	}
	return t.fake.Post(ctx, s)
}

func TestCaptureInRunsInDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("here"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, exit, err := CaptureIn(context.Background(), dir, "cat", "marker.txt")
	if err != nil || exit != 0 || out != "here" {
		t.Fatalf("out=%q exit=%d err=%v", out, exit, err)
	}
	if _, exit, _ := Capture(context.Background(), "cat", "marker.txt"); exit == 0 {
		t.Fatal("Capture without a dir must not see the repository files")
	}
}

// TestNestedProjectPath proves D-1345: a GitLab project in a subgroup travels as Owner "group" and Repo
// "sub/project" and validates, a malformed path never does, and the GitHub reporter (one owner, one repo)
// refuses a nested Repo before it builds an endpoint.
func TestNestedProjectPath(t *testing.T) {
	ok := Status{Owner: "group", Repo: "sub/project", Revision: sha, State: Success}
	if err := ok.Validate(); err != nil {
		t.Fatalf("nested project refused: %v", err)
	}
	deep := strings.TrimSuffix(strings.Repeat("s/", 18), "/")
	if err := (Status{Owner: "g", Repo: deep + "/p", Revision: sha, State: Success}).Validate(); err != nil {
		t.Fatalf("20 segments refused: %v", err)
	}
	for _, repo := range []string{"sub//p", "sub/../p", "sub/", "/p", "sub/.hidden", deep + "/s/p"} {
		if err := (Status{Owner: "g", Repo: repo, Revision: sha, State: Success}).Validate(); err == nil {
			t.Errorf("repo %q accepted", repo)
		}
	}
	called := false
	gh := GH{Capture: func(context.Context, ...string) (string, int, error) { called = true; return "", 0, nil }}
	if err := gh.Post(context.Background(), ok); err == nil || called {
		t.Errorf("GitHub reporter accepted a nested repo (err=%v, ran=%v)", err, called)
	}
}

// TestTruncateStripsLineBreaks proves D-1345: a carriage return is a line break too.
func TestTruncateStripsLineBreaks(t *testing.T) {
	if got := Truncate("a\r\nb\rc\nd"); strings.ContainsAny(got, "\r\n") {
		t.Fatalf("line break kept: %q", got)
	}
}
