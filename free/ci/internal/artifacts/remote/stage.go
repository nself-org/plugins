package remote

// Purpose: the node's side of artifact delivery (Epic CACHE D6/D7), kept here so
// the agent hook and the simulation tests run the same code. A Stage collects a
// finished job's declared outputs from its pinned workspace into a private blob
// store (records.Collector: path sanitiser, secret scan, size caps), then ships
// every blob and one artifact-ref per file over a carrier.
// Inputs:  the pinned workspace root and the job, a secret scanner, the caps in
// the environment (NSELF_CI_ARTIFACT_*), a carrier Uploader and its offset answers.
// Outputs: digest, chunk and artifact-ref frames; an E712 refusal for a declared
// output that is missing, too large, empty or holds a secret value.
// Constraints: collection runs in the agent process, never under the job's UID;
// an empty file is refused (the carriers announce blobs of one byte or more); a
// blob of a given digest is sent once however many files carry it; the staging
// directory belongs to one lease and Close removes it.

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Uploader is the sending side of a carrier (session.Uploader satisfies it).
type Uploader interface {
	transfer.Uploader
	ArtifactRef(protocol.ArtifactRef) error
}

// Stage is the collected outputs of one lease.
type Stage struct {
	dir   string
	lease string
	blobs *cas.Store
	realm cas.Realm

	mu     sync.Mutex
	staged []records.Staged
}

// NewStage opens a private blob store in dir (created, mode 0700) for one lease.
func NewStage(dir, lease string) (*Stage, error) {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, fmt.Errorf("artifacts: staging directory: %w", err)
	}
	blobs, err := cas.New(dir)
	if err != nil {
		return nil, fmt.Errorf("artifacts: staging store: %w", err)
	}
	return &Stage{dir: dir, lease: lease, blobs: blobs, realm: cas.ArtifactRealm(lease)}, nil
}

// Declares reports whether a job declares outputs, explicitly or by kind.
func Declares(job model.Job) bool {
	return len(job.Artifacts) > 0 || len(records.ImplicitDecls(job)) > 0
}

// Collect stores the declared outputs found in workspace. It is the executor's AfterJob.
func (s *Stage) Collect(ctx context.Context, workspace *os.Root, job model.Job, scanner authz.ValueScanner) error {
	decls, err := records.Normalize(job, job.Kind == "release", records.ImplicitDecls(job))
	if err != nil || len(decls) == 0 {
		return err
	}
	root, closeRoot, err := workdirRoot(workspace, job.Workdir)
	if err != nil {
		return err
	}
	defer closeRoot()
	pol, err := records.PolicyFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	pol.StorePath = s.dir
	staged, err := records.Collector{Blobs: s.blobs, Scanner: scanner, Policy: pol}.CollectRoot(ctx, s.lease, root, decls)
	if err != nil {
		return err
	}
	for _, f := range staged {
		if f.Size < 1 {
			return records.Failf(records.ReasonDeclaration, "%s is empty: a remote node cannot deliver a zero-byte artifact", f.Rel)
		}
	}
	s.mu.Lock()
	s.staged = staged
	s.mu.Unlock()
	return nil
}

// Files is how many files Collect stored.
func (s *Stage) Files() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.staged)
}

// Ship sends every collected blob and its artifact-ref. offsets carries the receiver's answers.
func (s *Stage) Ship(ctx context.Context, offsets <-chan protocol.Offset, up Uploader) error {
	s.mu.Lock()
	staged := s.staged
	s.mu.Unlock()
	send := &transfer.Sender{Up: up, Offsets: offsets}
	sent := map[string]bool{}
	for _, f := range staged {
		if !sent[f.SHA256] {
			sha := f.SHA256
			err := send.Send(ctx, transfer.Blob{Stream: transfer.StreamArtifact, SHA256: sha, Size: f.Size,
				Open: func() (io.ReadCloser, error) { return s.blobs.Open(s.realm, sha) }})
			if err != nil {
				return err
			}
			sent[sha] = true
		}
		if err := up.ArtifactRef(protocol.ArtifactRef{Path: f.Rel, SHA256: f.SHA256, Size: f.Size}); err != nil {
			return fmt.Errorf("artifacts: artifact-ref for %s: %w", f.Rel, err)
		}
	}
	return nil
}

// Close removes the staging directory.
func (s *Stage) Close() error { return os.RemoveAll(s.dir) }

// workdirRoot returns the job workdir inside the pinned workspace: relative, inside, not a symlink.
func workdirRoot(workspace *os.Root, workdir string) (*os.Root, func(), error) {
	if workdir == "" {
		return workspace, func() {}, nil
	}
	info, err := workspace.Lstat(workdir)
	if err != nil || !info.IsDir() {
		return nil, nil, records.Failf(records.ReasonUnreadable, "job workdir %q is missing, not a directory or a symlink", workdir)
	}
	sub, err := workspace.OpenRoot(workdir)
	if err != nil {
		return nil, nil, records.Failf(records.ReasonUnreadable, "job workdir %q escapes the workspace: %v", workdir, err)
	}
	return sub, func() { _ = sub.Close() }, nil
}
