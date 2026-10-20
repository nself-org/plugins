package remote

// Purpose: stage the node's entire planned save set before sending any frame.
// Inputs: pinned cache directories, the lease plan and registered-value scanner.
// Outputs: compressed, scanned cache blobs and cache-ref frames.
// Constraints: reuse specs' scan (including gzip payloads), sanitiser and caps;
// the manifest collector below is private staging, never coordinator persistence.

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/blob/transfer"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type CacheUploader interface {
	transfer.Uploader
	CacheRef(protocol.CacheRef) error
}

type NodeStage struct {
	WaitOffset time.Duration
	// Copies publishes the compressed wire blobs before a successful save is
	// advertised as local to this node. It is agent-owned, outside job mounts.
	Copies *ReadThrough
	dir    string
	lease  protocol.Lease
	blobs  *cas.Store
	realm  cas.Realm
	refs   []protocol.CacheRef
}

func NewNodeStage(dir string, lease protocol.Lease) (*NodeStage, error) {
	if lease.Cache == nil || lease.Cache.Access != "rw" {
		return nil, &entries.Refused{Reason: entries.ReasonNotPlanned}
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	b, err := cas.New(dir)
	if err != nil {
		return nil, err
	}
	return &NodeStage{dir: dir, lease: lease, blobs: b, realm: cas.CacheRealm(lease.Cache.Namespace)}, nil
}

// Collect reads the plan's dirs relative to base. No frames leave the node here.
func (s *NodeStage) Collect(ctx context.Context, base *os.Root, scanner authz.ValueScanner) (err error) {
	s.refs = nil
	defer func() {
		if err != nil {
			s.refs = nil
		}
	}()
	if s.lease.Cache == nil || s.lease.Cache.Access != "rw" {
		return nil
	}
	ns, err := DecodePlanNamespace(s.lease.Cache.Namespace)
	if err != nil {
		return err
	}
	if ns.Trust == "untrusted" || scanner == nil {
		return &entries.Refused{Reason: entries.ReasonNotPlanned}
	}
	collector := &nodeManifest{}
	es := entries.New(collector, s.blobs, entries.Options{DisableFloor: true})
	engine := specs.Engine{Entries: es, Blobs: s.blobs, Scanner: scanner, Backend: collector}
	for _, save := range s.lease.Cache.Save {
		sp, ok := specs.Get(save.Spec)
		if !ok {
			return &entries.Refused{Reason: entries.ReasonNotPlanned}
		}
		dirs := specs.Dirs{}
		for _, d := range sp.Dirs {
			dirs[d.Name] = path.Join(save.Spec, d.Name)
		}
		if len(save.Dirs) != len(dirs) {
			return &entries.Refused{Reason: entries.ReasonNotPlanned}
		}
		for _, d := range save.Dirs {
			if d != path.Join(save.Spec, path.Base(d)) || dirs[path.Base(d)] != d {
				return &entries.Refused{Reason: entries.ReasonNotPlanned}
			}
		}
		st, err := engine.Stage(ctx, specs.SaveRequest{Base: base, Attempt: s.lease.AttemptID, Namespace: ns, Key: save.Key, Spec: sp, Dirs: dirs})
		if errors.Is(err, specs.ErrEmpty) {
			continue
		}
		if err != nil {
			s.refs = nil
			return err
		}
		if _, err := engine.Commit(ctx, st, int64(s.lease.Epoch)); err != nil {
			return err
		}
		ref := protocol.CacheRef{LeaseID: s.lease.LeaseID, Spec: save.Spec, Key: save.Key}
		for _, blob := range collector.manifest.Blobs {
			sha, size, err := s.compress(ctx, cas.CacheRealm(string(ns.Canonical())), blob.SHA256)
			if err != nil {
				return err
			}
			ref.Blobs = append(ref.Blobs, protocol.CacheBlob{Path: blob.Path, SHA256: sha, Size: size})
		}
		s.refs = append(s.refs, ref)
	}
	return nil
}

func (s *NodeStage) compress(ctx context.Context, realm cas.Realm, digest string) (string, int64, error) {
	r, err := s.blobs.Open(realm, digest)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = r.Close() }()
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		gz, _ := gzip.NewWriterLevel(pw, gzip.BestSpeed)
		_, err := io.Copy(gz, r)
		if cerr := gz.Close(); err == nil {
			err = cerr
		}
		_ = pw.CloseWithError(err)
	}()
	sha, size, err := s.blobs.Put(ctx, s.realm, pr)
	_ = pr.CloseWithError(err)
	<-done
	return sha, size, err
}

func (s *NodeStage) Ship(ctx context.Context, offsets <-chan protocol.Offset, up CacheUploader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.lease.Cache == nil || s.lease.Cache.Access != "rw" {
		return fmt.Errorf("E711 not_planned: read-only node save")
	}
	if err := s.publishCopies(ctx); err != nil {
		return err
	}
	sender := transfer.Sender{Up: up, Offsets: offsets, WaitOffset: s.WaitOffset}
	sent := map[string]bool{}
	for _, ref := range s.refs {
		for _, b := range ref.Blobs {
			if sent[b.SHA256] {
				continue
			}
			sha := b.SHA256
			if err := sender.Send(ctx, transfer.Blob{Stream: "cache", SHA256: sha, Size: b.Size, Open: func() (io.ReadCloser, error) { return s.blobs.Open(s.realm, sha) }}); err != nil {
				return err
			}
			sent[sha] = true
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := up.CacheRef(ref); err != nil {
			return err
		}
	}
	return nil
}

func (s *NodeStage) publishCopies(ctx context.Context) error {
	if s.Copies == nil {
		return nil
	}
	var total int64
	seen := map[string]bool{}
	for _, ref := range s.refs {
		for _, b := range ref.Blobs {
			if seen[b.SHA256] {
				continue
			}
			seen[b.SHA256] = true
			if b.Size < 0 || b.Size > s.Copies.max-total {
				return &entries.Refused{Reason: entries.ReasonSizeCap}
			}
			total += b.Size
		}
	}
	for _, ref := range s.refs {
		for _, b := range ref.Blobs {
			f, err := s.Copies.Open(ctx, s.lease.Cache.Namespace, b.SHA256, b.Size, func(ctx context.Context, dst io.Writer) error {
				src, err := s.blobs.Open(s.realm, b.SHA256)
				if err != nil {
					return err
				}
				defer src.Close()
				_, err = io.Copy(dst, &contextReader{ctx: ctx, r: src})
				return err
			})
			if err != nil {
				return err
			}
			_ = f.Close()
		}
	}
	return nil
}

func (s *NodeStage) Close() error { return os.RemoveAll(s.dir) }

// nodeManifest captures the manifest produced by the reusable specs engine.
// Its embedded interfaces are never used: no job facts or entry rows exist on a node.
type nodeManifest struct {
	entries.Backend
	manifest entries.Manifest
}

func (n *nodeManifest) CacheCommit(_ context.Context, c entries.CommitRequest) error {
	if err := c.Check(); err != nil {
		return err
	}
	n.manifest = c.Manifest
	return nil
}
func (n *nodeManifest) AppendAudit(context.Context, string, string, string, string) error { return nil }
func (n *nodeManifest) AttemptStateEpoch(context.Context, string) (model.JobState, int64, error) {
	return "finalizing", 0, nil
}
