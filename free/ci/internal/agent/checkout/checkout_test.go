package checkout

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// tdir keeps every test file in the module's scratch convention (t.TempDir).
func makeRepo(t *testing.T) (repo, rev string) {
	t.Helper()
	repo = t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "hello.txt"), []byte("hello bundle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "hello.txt")
	run("commit", "-q", "-m", "one")
	rev = run("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "later.txt"), []byte("not in the lease\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "later.txt")
	run("commit", "-q", "-m", "two")
	return repo, rev
}

func bundleFor(t *testing.T, repo, rev string) (data []byte, digest string) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "out.bundle")
	digest, size, err := Create(context.Background(), nil, repo, rev, dest)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(dest)
	if err != nil || int64(len(data)) != size {
		t.Fatalf("read bundle: %v (%d vs %d)", err, len(data), size)
	}
	return data, digest
}

func feedAll(t *testing.T, r *Receiver, l protocol.Lease, data []byte, step int) error {
	t.Helper()
	var err error
	for off := 0; off < len(data); off += step {
		end := min(off+step, len(data))
		err = r.Feed(l.LeaseID, l.Epoch, protocol.Chunk{Stream: Stream, Offset: int64(off), Data: base64.StdEncoding.EncodeToString(data[off:end]), Final: end == len(data)})
		if err != nil {
			return err
		}
	}
	return err
}

func leaseFor(rev, digest string, size int) protocol.Lease {
	return protocol.Lease{LeaseID: "lease/1", Epoch: 3, Checkout: protocol.Checkout{Revision: rev, BundleDigest: digest, Size: int64(size)}}
}

func TestBundleRoundTripStagesExactRevision(t *testing.T) {
	repo, rev := makeRepo(t)
	data, digest := bundleFor(t, repo, rev)
	if refs, _ := exec.Command("git", "-C", repo, "for-each-ref", "refs/nself-ci").Output(); len(refs) != 0 {
		t.Fatalf("Create left a pinned ref behind: %s", refs)
	}
	base := filepath.Join(t.TempDir(), "stage")
	r := NewReceiver(base, nil)
	l := leaseFor(rev, digest, len(data))
	if err := r.Expect(l); err != nil {
		t.Fatal(err)
	}
	if err := feedAll(t, r, l, data, 1000); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	src, err := r.Stage(context.Background(), l, root)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(src, "hello.txt")); err != nil || string(b) != "hello bundle\n" {
		t.Fatalf("hello.txt = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(src, "later.txt")); err == nil {
		t.Fatal("the checkout holds a file from a later commit")
	}
	if entries, _ := os.ReadDir(base); len(entries) != 0 {
		t.Fatalf("staging root not removed: %v", entries)
	}
}

func TestDigestMismatchRefusedBeforeAnyGit(t *testing.T) {
	repo, rev := makeRepo(t)
	data, digest := bundleFor(t, repo, rev)
	data[len(data)/2] ^= 0xff // one flipped byte
	called := false
	r := NewReceiver(filepath.Join(t.TempDir(), "stage"), func(context.Context, ...string) (string, int, error) {
		called = true
		return "", 0, nil
	})
	l := leaseFor(rev, digest, len(data))
	if err := r.Expect(l); err != nil {
		t.Fatal(err)
	}
	if err := feedAll(t, r, l, data, 4096); !errors.Is(err, ErrDigest) {
		t.Fatalf("Feed err = %v, want ErrDigest", err)
	}
	if _, err := r.Stage(context.Background(), l, t.TempDir()); !errors.Is(err, ErrDigest) {
		t.Fatalf("Stage err = %v, want ErrDigest", err)
	}
	if called {
		t.Fatal("a git process ran on a bundle whose digest did not match")
	}
}

func TestFeedFailsClosed(t *testing.T) {
	_, rev := makeRepo(t)
	digest := "sha256:" + strings.Repeat("ab", 32)
	chunk := func(off int64, data string, final bool) protocol.Chunk {
		return protocol.Chunk{Stream: Stream, Offset: off, Data: data, Final: final}
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("0123456789"))
	cases := []struct {
		name  string
		size  int
		feed  func(*Receiver, protocol.Lease) error
		match error
	}{
		{"gap", 20, func(r *Receiver, l protocol.Lease) error { return r.Feed(l.LeaseID, l.Epoch, chunk(5, b64, false)) }, ErrOrder},
		{"too large", 5, func(r *Receiver, l protocol.Lease) error { return r.Feed(l.LeaseID, l.Epoch, chunk(0, b64, false)) }, ErrTooLarge},
		{"short", 20, func(r *Receiver, l protocol.Lease) error { return r.Feed(l.LeaseID, l.Epoch, chunk(0, b64, true)) }, ErrIncomplete},
		{"other epoch", 20, func(r *Receiver, l protocol.Lease) error { return r.Feed(l.LeaseID, l.Epoch+1, chunk(0, b64, false)) }, ErrUnexpected},
		{"other lease", 20, func(r *Receiver, l protocol.Lease) error { return r.Feed("nope", l.Epoch, chunk(0, b64, false)) }, ErrUnexpected},
		{"wrong stream", 20, func(r *Receiver, l protocol.Lease) error {
			return r.Feed(l.LeaseID, l.Epoch, protocol.Chunk{Stream: "cache", Digest: "x", Data: b64})
		}, ErrUnexpected},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
			l := leaseFor(rev, digest, c.size)
			if err := r.Expect(l); err != nil {
				t.Fatal(err)
			}
			if err := c.feed(r, l); !errors.Is(err, c.match) {
				t.Fatalf("err = %v, want %v", err, c.match)
			}
		})
	}
	t.Run("not base64", func(t *testing.T) {
		r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
		l := leaseFor(rev, digest, 20)
		if err := r.Expect(l); err != nil {
			t.Fatal(err)
		}
		if err := r.Feed(l.LeaseID, l.Epoch, chunk(0, "!!not base64!!", false)); err == nil {
			t.Fatal("accepted a chunk that is not base64")
		}
		if _, err := r.Stage(context.Background(), l, t.TempDir()); err == nil {
			t.Fatal("Stage succeeded after a failed transfer")
		}
	})
}

func TestExpectRefusesBadLeases(t *testing.T) {
	_, rev := makeRepo(t)
	r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
	good := "sha256:" + strings.Repeat("0", 64)
	for name, l := range map[string]protocol.Lease{
		"huge":          leaseFor(rev, good, int(MaxBundle)+1),
		"short rev":     leaseFor("abc123", good, 10),
		"bad digest":    leaseFor(rev, "sha256:zz", 10),
		"option as rev": leaseFor("--upload-pack=x", good, 10),
	} {
		if err := r.Expect(l); err == nil {
			t.Errorf("%s: Expect accepted the lease", name)
		}
	}
	if err := r.Expect(leaseFor(rev, good, 0)); err != nil {
		t.Fatalf("lease without a checkout: %v", err)
	}
}

func TestCreateRefusesBadRevision(t *testing.T) {
	repo, _ := makeRepo(t)
	for _, rev := range []string{"", "main", "HEAD", "--all", strings.Repeat("g", 40)} {
		if _, _, err := Create(context.Background(), nil, repo, rev, filepath.Join(t.TempDir(), "b")); err == nil {
			t.Errorf("Create accepted revision %q", rev)
		}
	}
}
