package toolchain

// Purpose: name the toolchain image `nself ci --container` runs checks in.
// Inputs:  NSELF_CI_IMAGE (the value of --image: ref@digest), the
// toolchain-image row of versions.yaml (embedded; NSELF_CI_TOOLCHAIN_VERSIONS
// names another table, for fixtures and air-gapped mirrors), a docker runner.
// Outputs: the pinned image reference ref@sha256:digest, present on this host.
// Constraints: no silent fallback (Epic D24): a pending row, a malformed
// override or a digest that cannot be pulled is E645; a reference by tag is
// never accepted; the override comes from the operator's own environment, never
// from job env.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Environment variables the resolver reads.
const (
	EnvImage    = "NSELF_CI_IMAGE"
	EnvVersions = "NSELF_CI_TOOLCHAIN_VERSIONS"
)

// refRe is a docker reference pinned by digest: [host[:port]/]repo[:tag]@sha256:hex.
var refRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:-]*@sha256:[0-9a-f]{64}$`)

// Capture runs argv and returns its combined output and exit code; err is set
// only when the process could not run (the doctor and report funnel).
type Capture func(ctx context.Context, argv ...string) (string, int, error)

// Resolver turns the environment and the pins into an image reference.
type Resolver struct {
	Getenv   func(string) string
	ReadFile func(string) ([]byte, error)
	Capture  Capture // nil: do not check or pull the image

	mu   sync.Mutex
	have map[string]bool
}

// NewResolver reads the real process environment and uses capture for docker.
func NewResolver(capture Capture) *Resolver {
	return &Resolver{Getenv: os.Getenv, ReadFile: os.ReadFile, Capture: capture}
}

// Resolve returns the image reference to run in, pulled when it is absent.
func (r *Resolver) Resolve(ctx context.Context) (string, error) {
	ref, err := r.reference()
	if err != nil {
		return "", err
	}
	if err := r.ensure(ctx, ref); err != nil {
		return "", err
	}
	return ref, nil
}

func (r *Resolver) reference() (string, error) {
	if v := strings.TrimSpace(r.Getenv(EnvImage)); v != "" {
		if !refRe.MatchString(v) {
			return "", fail("--image %q is not a reference pinned by digest (ref@sha256:<64 hex>)", v)
		}
		return v, nil
	}
	table, err := r.table()
	if err != nil {
		return "", err
	}
	row, ok := table.Row(ImageRow)
	if !ok {
		return "", fail("versions.yaml has no %s row", ImageRow)
	}
	if row.Status != StatusPinned {
		return "", fail("the pinned toolchain image %s is not published yet (row %s is %s); pass --image <ref@digest> (or set NSELF_CI_IMAGE=<ref@digest>) or run without --container", row.Image, ImageRow, row.Status)
	}
	ref := row.Image + "@" + row.Digest
	if !refRe.MatchString(ref) {
		return "", fail("row %s names %q, which is not a reference pinned by digest", ImageRow, ref)
	}
	return ref, nil
}

func (r *Resolver) table() (Versions, error) {
	path := strings.TrimSpace(r.Getenv(EnvVersions))
	if path == "" {
		return Embedded()
	}
	b, err := r.ReadFile(path)
	if err != nil {
		return Versions{}, fail("cannot read %s=%s: %v", EnvVersions, path, err)
	}
	return ParseVersions(b)
}

// ensure makes ref present on the host. docker verifies the manifest digest
// while pulling, so content that does not match the pin never lands.
func (r *Resolver) ensure(ctx context.Context, ref string) error {
	if r.Capture == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.have[ref] {
		return nil
	}
	if _, rc, err := r.Capture(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", ref); err == nil && rc == 0 {
		return r.remember(ref)
	}
	out, rc, err := r.Capture(ctx, "docker", "pull", "--quiet", ref)
	if err != nil || rc != 0 {
		return fail("cannot pull %s (digest mismatch, unknown digest or registry unreachable): %s", ref, tail(out, err))
	}
	return r.remember(ref)
}

func (r *Resolver) remember(ref string) error {
	if r.have == nil {
		r.have = map[string]bool{}
	}
	r.have[ref] = true
	return nil
}

func tail(out string, err error) string {
	out = strings.TrimSpace(out)
	if err != nil {
		out = err.Error()
	}
	if len(out) > 300 {
		out = out[len(out)-300:]
	}
	return out
}
