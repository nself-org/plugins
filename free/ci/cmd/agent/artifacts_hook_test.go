package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// recUp is a carrier end that records frames and answers every digest with offset 0.
type recUp struct {
	kinds   []string
	refs    []protocol.ArtifactRef
	offsets chan protocol.Offset
}

func newRecUp() *recUp { return &recUp{offsets: make(chan protocol.Offset, 16)} }

func (u *recUp) Digest(d protocol.Digest) error {
	u.kinds = append(u.kinds, "digest")
	u.offsets <- protocol.Offset{Stream: d.Stream, SHA256: d.SHA256, Offset: 0}
	return nil
}
func (u *recUp) Offset(protocol.Offset) error { return nil }
func (u *recUp) Chunk(protocol.Chunk) error   { u.kinds = append(u.kinds, "chunk"); return nil }
func (u *recUp) ArtifactRef(a protocol.ArtifactRef) error {
	u.kinds = append(u.kinds, "ref")
	u.refs = append(u.refs, a)
	return nil
}
func (u *recUp) CacheRef(protocol.CacheRef) error { return nil }
func (u *recUp) Offsets() <-chan protocol.Offset  { return u.offsets }

var _ session.OffsetSource = (*recUp)(nil)

func hookLease(artifacts ...string) protocol.Lease {
	j := model.Job{Run: []string{"true"}}
	for _, a := range artifacts {
		j.Artifacts = append(j.Artifacts, model.ArtifactDecl{Path: a})
	}
	return protocol.Lease{LeaseID: "lease-h1", AttemptID: "att-h1", Epoch: 1, Job: j}
}

func workspaceWith(t *testing.T, files map[string]string) *os.Root {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestArtifactsHookIgnoresALeaseThatDeclaresNothing(t *testing.T) {
	root := session.JobRoot{Dir: t.TempDir()}
	l := hookLease()
	if _, err := artifactsBefore(context.Background(), l, root); err != nil {
		t.Fatal(err)
	}
	if f := artifactsAfterJob(execSpec{AttemptID: l.AttemptID, Job: l.Job}, jobInputs{}); f != nil {
		t.Fatal("an AfterJob was installed for a job with no artifacts")
	}
	if err := artifactsAfter(context.Background(), l, root, session.Outcome{State: "passed"}, newRecUp()); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactsHookCollectsInTheWorkspaceAndShipsAfterAPass(t *testing.T) {
	l := hookLease("out/*.txt")
	root := session.JobRoot{Dir: t.TempDir()}
	if _, err := artifactsBefore(context.Background(), l, root); err != nil {
		t.Fatal(err)
	}
	ws := workspaceWith(t, map[string]string{"out/a.txt": "alpha", "out/b.txt": "beta", "out/c.bin": "ignored"})
	after := artifactsAfterJob(execSpec{AttemptID: l.AttemptID, Job: l.Job}, jobInputs{})
	if after == nil {
		t.Fatal("no AfterJob for a job that declares artifacts")
	}
	if err := after(context.Background(), ws); err != nil {
		t.Fatal(err)
	}
	up := newRecUp()
	if err := artifactsAfter(context.Background(), l, root, session.Outcome{State: "passed"}, up); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(up.kinds, ","); got != "digest,chunk,ref,digest,chunk,ref" {
		t.Fatalf("frames = %s", got)
	}
	if len(up.refs) != 2 || up.refs[0].Path != "out/a.txt" || up.refs[1].Path != "out/b.txt" {
		t.Fatalf("refs = %+v", up.refs)
	}
	if _, err := os.Stat(filepath.Join(root.Dir, "artifacts-stage")); !os.IsNotExist(err) {
		t.Fatalf("staging directory left behind: %v", err)
	}
}

func TestArtifactsHookSendsNothingAfterAFailureAndFailsOnMissingOutputs(t *testing.T) {
	l := hookLease("out/a.txt")
	root := session.JobRoot{Dir: t.TempDir()}
	if _, err := artifactsBefore(context.Background(), l, root); err != nil {
		t.Fatal(err)
	}
	after := artifactsAfterJob(execSpec{AttemptID: l.AttemptID, Job: l.Job}, jobInputs{})
	if err := after(context.Background(), workspaceWith(t, map[string]string{"other.txt": "x"})); err == nil || !strings.Contains(err.Error(), "E712") {
		t.Fatalf("missing output: %v, want E712", err)
	}
	up := newRecUp()
	if err := artifactsAfter(context.Background(), l, root, session.Outcome{State: "failed"}, up); err != nil || len(up.kinds) != 0 {
		t.Fatalf("after a failure: %v %v", err, up.kinds)
	}
}

func TestArtifactsHookRefusesEmptyFilesAndSecretValues(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		in    jobInputs
		want  string
	}{
		"empty":  {map[string]string{"out/a.txt": ""}, jobInputs{}, "zero-byte"},
		"secret": {map[string]string{"out/a.txt": "token=hunter2-hunter2-hunter2"}, jobInputs{Secrets: map[string]string{"T": "hunter2-hunter2-hunter2"}}, "E712"},
	} {
		t.Run(name, func(t *testing.T) {
			l := hookLease("out/a.txt")
			l.AttemptID = "att-" + name
			root := session.JobRoot{Dir: t.TempDir()}
			if _, err := artifactsBefore(context.Background(), l, root); err != nil {
				t.Fatal(err)
			}
			err := artifactsAfterJob(execSpec{AttemptID: l.AttemptID, Job: l.Job}, tc.in)(context.Background(), workspaceWith(t, tc.files))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			_ = artifactsAfter(context.Background(), l, root, session.Outcome{State: "failed"}, newRecUp())
		})
	}
}

// brokenUp is a carrier whose digest frame cannot be sent: the shipping fails part-way.
type brokenUp struct{ *recUp }

func (brokenUp) Digest(protocol.Digest) error { return errors.New("carrier cut") }

// A failed delivery must reach the session as ErrHookFatal, or the job would pass with part of its outputs.
func TestArtifactsAfterWrapsAFailedShipAsHookFatal(t *testing.T) {
	l := hookLease("out/*.txt")
	root := session.JobRoot{Dir: t.TempDir()}
	if _, err := artifactsBefore(context.Background(), l, root); err != nil {
		t.Fatal(err)
	}
	after := artifactsAfterJob(execSpec{AttemptID: l.AttemptID, Job: l.Job}, jobInputs{})
	if err := after(context.Background(), workspaceWith(t, map[string]string{"out/a.txt": "alpha"})); err != nil {
		t.Fatal(err)
	}
	err := artifactsAfter(context.Background(), l, root, session.Outcome{State: "passed"}, brokenUp{newRecUp()})
	if !errors.Is(err, session.ErrHookFatal) {
		t.Fatalf("err = %v, want ErrHookFatal", err)
	}
}
