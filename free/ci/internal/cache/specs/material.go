package specs

// Purpose: the key material of one spec for one attempt (Epic CACHE D3):
// everything in the cache key except the namespace, so the same material keys
// the job's own namespace and each snapshot namespace it may read.
// Inputs:  Input (spec, platform, isolation, image digest, narrowed detector
// facts, extra keys, environment, a tool probe).
// Outputs: Material, Material.Key, EnvDigest, ErrUnavailable.
// Constraints: environment values are hashed, never kept; the probe runs only
// for process-level jobs (the image digest is the toolchain identity of a
// container job); a failed probe makes the spec unavailable rather than keyed
// by a guess.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
)

// ErrUnavailable means the spec cannot be keyed now (a probe failed, or the
// toolchain identity is unknown); the caller records a disabled outcome.
var ErrUnavailable = errors.New("specs: toolchain identity unavailable")

// Input is what Resolve needs. Facts are already narrowed by cache.paths.
type Input struct {
	Spec      Spec
	Platform  string
	Isolation string
	Image     string // pinned image digest for container levels, else ""
	Facts     []detect.Fact
	Scope     []string // the narrowed cache.paths, sorted
	ExtraKeys []string
	Environ   []string // NAME=VALUE entries the job sees (job env over process env)
	Probe     func(ctx context.Context, argv []string) (string, error)
}

// Material is the namespace-free part of a key.
type Material struct {
	Spec      Spec
	Platform  string
	Isolation string
	Toolchain []string
	EnvDigest string
	Paths     []string
	Lockfiles []string
	Extras    []string
}

// Resolve computes the material, running the probes of a process-level job.
func Resolve(ctx context.Context, in Input) (Material, error) {
	m := Material{Spec: in.Spec, Platform: in.Platform, Isolation: in.Isolation, EnvDigest: EnvDigest(in.Spec.EnvAllow, in.Environ),
		Extras: append([]string(nil), in.ExtraKeys...)}
	if in.Image != "" {
		m.Toolchain = []string{"image=" + in.Image}
	} else {
		if in.Isolation == "container" || in.Probe == nil && len(in.Spec.Probes) > 0 {
			return m, fmt.Errorf("%w: no image digest or probe for spec %s", ErrUnavailable, in.Spec.ID)
		}
		for _, p := range in.Spec.Probes {
			line, err := in.Probe(ctx, p.Argv)
			if line = firstLine(line); err != nil || line == "" {
				return m, fmt.Errorf("%w: probe %s failed", ErrUnavailable, p.Name)
			}
			m.Toolchain = append(m.Toolchain, p.Name+"="+line)
		}
	}
	sort.Strings(m.Toolchain)
	for _, d := range in.Spec.Dirs {
		m.Paths = append(m.Paths, in.Spec.ID+"/"+d.Name)
	}
	for _, s := range in.Scope {
		m.Paths = append(m.Paths, "scope:"+s)
	}
	for _, f := range in.Facts {
		if f.Kind == "lockfile" && f.Confidence == detect.Certain && f.SHA256 != "" && contains(in.Spec.Lockfiles, f.Name) {
			m.Lockfiles = append(m.Lockfiles, f.Path+"="+f.SHA256)
		}
	}
	return m, nil
}

// Key derives the cache key for ns (entries.Key sorts every list itself).
func (m Material) Key(ns authz.Namespace) (string, error) {
	return entries.Key(entries.KeyInputs{Namespace: ns, SpecID: m.Spec.ID, SpecVersion: m.Spec.Version, Platform: m.Platform,
		Isolation: m.Isolation, Toolchain: m.Toolchain, EnvDigest: m.EnvDigest, Paths: m.Paths, Lockfiles: m.Lockfiles, Extras: m.Extras})
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// allowed reports whether name is selected by an allowlist entry.
func allowed(allow []string, name string) bool {
	for _, a := range allow {
		if p, ok := strings.CutSuffix(a, "*"); ok && strings.HasPrefix(name, p) || a == name {
			return true
		}
	}
	return false
}

// EnvDigest is the sha256 over the allowlisted variables that are set: each
// name and value as a 4-byte big-endian length plus its bytes, sorted by name.
// An unset variable adds nothing, a variable set to "" adds its name. Only the
// digest leaves this function.
func EnvDigest(allow, environ []string) string {
	vals := map[string]string{}
	for _, kv := range environ {
		if name, v, ok := strings.Cut(kv, "="); ok && allowed(allow, name) {
			vals[name] = v
		}
	}
	names := make([]string, 0, len(vals))
	for n := range vals {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	put := func(b string) {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(b)))
		h.Write(n[:])
		h.Write([]byte(b))
	}
	for _, n := range names {
		put(n)
		put(vals[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}
