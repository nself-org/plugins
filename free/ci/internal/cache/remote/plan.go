// Package remote coordinates the cache plan and fenced saves of remote jobs.
package remote

// Purpose: derive the wire plan exclusively from coordinator detector and trust facts.
// Inputs: PlanInput and the authorized entries store. Outputs: lease cache plan.
// Constraints: untrusted work receives read-only access and no save keys.
import (
	"context"
	"encoding/base64"
	"path"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/cache/specs"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

type PlanInput struct {
	Entries                    *entries.Store
	Backend                    specs.Backend
	Facts                      entries.Facts
	Detected                   []detect.Fact
	Scope, ExtraKeys, Environ  []string
	Platform, Isolation, Image string
	Container                  bool
	Probe                      func(context.Context, []string) (string, error)
}

const planTimeout = 2 * time.Second

// BuildPlan bounds optional cache work within lease delivery; expiry is a miss.
func BuildPlan(ctx context.Context, in PlanInput) (*protocol.LeaseCache, error) {
	ctx, cancel := context.WithTimeout(ctx, planTimeout)
	defer cancel()
	p, err := buildPlan(ctx, in)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return p, err
}

// buildPlan performs exact-key lookups; a read-only lease has one snapshot realm.
func buildPlan(ctx context.Context, in PlanInput) (*protocol.LeaseCache, error) {
	switch in.Isolation {
	case "", "process", "container", "sandboxed-container":
	default:
		return nil, nil
	}
	own, err := entries.NamespaceOf(in.Facts)
	if err != nil {
		return nil, err
	}
	job := authz.Job{Namespace: own, State: authz.StateRunning}
	ns, access := own, authz.CacheAccess(job, own)
	if own.Trust == model.TrustClass("untrusted") {
		ns = authz.Namespace{Project: own.Project, Trust: "owner", Protection: authz.Protected}
		access = authz.CacheAccess(job, ns)
	}
	p := &protocol.LeaseCache{Namespace: base64.RawURLEncoding.EncodeToString(ns.Canonical()), Access: "rw"}
	if access == authz.ModeROSnapshot {
		p.Access = "ro"
	}
	if access == authz.ModeNone {
		return nil, nil
	}
	if p.Access == "ro" && !in.Container {
		return nil, nil
	}
	for _, sp := range specs.For(in.Detected, in.Container) {
		mat, e := specs.Resolve(ctx, specs.Input{Spec: sp, Platform: in.Platform, Isolation: in.Isolation, Image: in.Image, Facts: in.Detected,
			Scope: in.Scope, ExtraKeys: in.ExtraKeys, Environ: in.Environ, Probe: in.Probe})
		if e != nil {
			continue
		}
		key, e := mat.Key(ns)
		if e != nil {
			return nil, e
		}
		if in.Entries != nil {
			// The node verifies bytes while restoring; delivery only needs metadata.
			m, hit, e := in.Entries.LookupForRestore(ctx, ns, key, access)
			if e != nil {
				return nil, e
			}
			if hit && in.Backend != nil {
				state, _, err := in.Backend.AttemptStateEpoch(ctx, m.Writer.AttemptID)
				if err != nil {
					return nil, err
				}
				if state != "passed" {
					hit = false
				}
			} else {
				hit = false
			}
			if hit {
				r := protocol.CacheRestore{Spec: sp.ID, Key: key, Dir: sp.ID}
				for _, b := range m.Blobs {
					r.Blobs = append(r.Blobs, protocol.CacheBlob{Path: b.Path, SHA256: b.SHA256, Size: b.Size})
				}
				p.Restore = append(p.Restore, r)
				continue
			}
		}
		if access == authz.ModeRWOwn {
			s := protocol.CacheSave{Spec: sp.ID, Key: key}
			for _, d := range sp.Dirs {
				s.Dirs = append(s.Dirs, path.Join(sp.ID, d.Name))
			}
			p.Save = append(p.Save, s)
		}
	}
	if len(p.Restore)+len(p.Save) == 0 {
		return nil, nil
	}
	return p, nil
}
