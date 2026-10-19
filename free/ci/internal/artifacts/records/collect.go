package records

// Purpose: collect the declared outputs of one finished job from its workdir
// into the project realm of the blob store (Epic CACHE D6, R-SEC-18, R-SEC-29).
// Inputs:  the workdir, normalized declarations, the project, the blob store,
// the secret scanner and the Policy caps.
// Outputs: Staged values, one per stored file, each with its digest and size.
// Constraints: nothing leaves the workdir (os.Root, no symlink followed); every
// entry passes authz.ArtifactPath through an Extractor; a declaration that
// selects nothing, a file over the cap, a registered secret, a low disk and a
// duplicate record name each fail the whole collection (E712 or E683) before
// the attempt can pass; blobs are written only after the scan of their bytes.

import (
	"context"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

// Blobs is the part of the blob store the records use (*cas.Store).
type Blobs interface {
	Put(ctx context.Context, realm cas.Realm, src io.Reader) (string, int64, error)
	Open(realm cas.Realm, digest string) (io.ReadCloser, error)
	Stat(realm cas.Realm, digest string) (os.FileInfo, error)
	Sweep(ctx context.Context, kind string, keep func(cas.Realm, string) bool, now time.Time) error
}

// Staged is one stored file, not yet recorded.
type Staged struct {
	Decl   Decl
	Name   string
	Rel    string
	SHA256 string
	Size   int64
	Image  *ImageInfo
}

// Collector gathers declared outputs.
type Collector struct {
	Blobs   Blobs
	Scanner authz.ValueScanner
	Policy  Policy
	Limits  authz.Limits
	// Usage is what the store holds now (Ledger.ArtifactUsage); the declared
	// and streamed bytes are checked against the caps on top of it.
	Usage Usage
	// Reserve, when set, records the declared total in the store before the
	// first byte is written, so concurrent attempts see each other's claims.
	Reserve func(total int64) error
	// MaxRecords bounds the files one attempt may collect (0 = DefaultMaxRecords).
	MaxRecords int
}

// DefaultMaxRecords is the most files one attempt may collect across all its declarations.
const DefaultMaxRecords = 10000

// planned is one admitted file with the record name it will get.
type planned struct {
	decl Decl
	f    found
	name string
}

// Collect stores every file the declarations select under the directory base
// (opened here; callers that hold a pinned root use CollectRoot).
func (c Collector) Collect(ctx context.Context, project, base string, decls []Decl) ([]Staged, error) {
	if info, err := os.Lstat(base); err != nil || !info.IsDir() {
		return nil, failf(ReasonUnreadable, "job workdir %q is missing, not a directory or a symlink", base)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, failf(ReasonUnreadable, "job workdir: %v", err)
	}
	defer func() { _ = root.Close() }()
	return c.CollectRoot(ctx, project, root, decls)
}

// CollectRoot stores every file the declarations select under root. Every
// entry of every declaration is judged, and the declared bytes are checked
// against the store caps (Usage), before the first byte is stored, so a
// refusal leaves nothing behind; the bytes actually streamed are held to the
// same budget.
func (c Collector) CollectRoot(ctx context.Context, project string, root *os.Root, decls []Decl) ([]Staged, error) {
	plan, err := c.plan(ctx, root, decls)
	if err != nil {
		return nil, err
	}
	realm := cas.ArtifactRealm(project)
	out := make([]Staged, 0, len(plan))
	var streamed int64
	for _, p := range plan {
		st, err := c.store(ctx, root, realm, p, streamed)
		if err != nil {
			return nil, err
		}
		streamed += st.Size
		out = append(out, st)
	}
	return out, nil
}

// plan expands, sanitises and caps every declaration (no bytes are read).
func (c Collector) plan(ctx context.Context, root *os.Root, decls []Decl) ([]planned, error) {
	lim := c.Limits
	lim.MaxFileBytes, lim.MaxTotalBytes = math.MaxInt64/4, math.MaxInt64/4 // the caps below are E712; authz keeps the shape rules
	names := map[string]bool{}
	var total int64
	var out []planned
	budget := c.MaxRecords
	if budget <= 0 {
		budget = DefaultMaxRecords
	}
	for _, d := range decls {
		ents, err := expand(ctx, root.FS(), d.Path, budget-len(out))
		if err != nil {
			return nil, err
		}
		if len(out)+len(ents) > budget { // the budget spans declarations, whichever way a declaration selects
			return nil, failf(ReasonSizeCap, "the declared outputs select more than %d files", budget)
		}
		if len(ents) == 0 {
			return nil, failf(ReasonMissing, "declared artifact %q matched no file in the workdir", d.Path)
		}
		if d.Kind == KindImage && len(ents) != 1 {
			return nil, failf(ReasonImage, "artifact %q of kind image must select exactly one OCI layout tarball, it selects %d files", d.Path, len(ents))
		}
		x := authz.NewExtractor(lim)
		literal := !hasMeta(d.Path) && len(ents) == 1 && ents[0].rel == d.Path
		for _, f := range ents {
			name := recordName(d, f.rel, literal)
			if c.Scanner != nil && (len(c.Scanner.Scan([]byte(name))) != 0 || len(c.Scanner.Scan([]byte(f.rel))) != 0) {
				return nil, failf(ReasonSecretMatch, "an artifact name or path holds a registered secret value; nothing is stored") // judged first: every later message names the path
			}
			if f.info.Mode().IsRegular() {
				if err = c.Policy.CheckFile(name, f.info.Size()); err != nil {
					return nil, err
				}
				if total += f.info.Size(); total > c.Policy.MaxTotal {
					return nil, failf(ReasonSizeCap, "the declared outputs exceed NSELF_CI_ARTIFACT_MAX_BYTES (%d B)", c.Policy.MaxTotal)
				}
			}
			if err = admit(x, root.FS(), f); err != nil {
				return nil, err
			}
			if names[name] {
				return nil, failf(ReasonDuplicate, "two declarations produce the artifact name %q", name)
			}
			names[name] = true
			out = append(out, planned{d, f, name})
		}
	}
	if _, err := c.Policy.CheckCaps(c.Usage, total); err != nil {
		return nil, err
	}
	if c.Reserve != nil { // hold the declared bytes in the store before any is written
		if err := c.Reserve(total); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// recordName: no name = the workdir-relative path; a name on a single literal
// file = the name; a name on a glob or directory = name/relative-path.
func recordName(d Decl, rel string, literal bool) string {
	if d.Name == "" {
		return rel
	}
	if literal {
		return d.Name
	}
	return d.Name + "/" + strings.TrimPrefix(rel, strings.TrimSuffix(literalPrefix(d.Path), "/")+"/")
}

// literalPrefix is the leading components of a pattern that hold no wildcard.
func literalPrefix(p string) string {
	var keep []string
	for _, c := range strings.Split(p, "/") {
		if hasMeta(c) {
			break
		}
		keep = append(keep, c)
	}
	return strings.Join(keep, "/")
}

func (c Collector) store(ctx context.Context, root *os.Root, realm cas.Realm, p planned, streamed int64) (Staged, error) {
	d, f, name := p.decl, p.f, p.name
	if err := c.Policy.CheckDisk(f.info.Size()); err != nil {
		return Staged{}, err
	}
	file, err := root.Open(f.rel)
	if err != nil {
		return Staged{}, failf(ReasonUnreadable, "%s: %v", f.rel, err)
	}
	defer func() { _ = file.Close() }()
	if now, err := file.Stat(); err != nil || !os.SameFile(now, f.info) || !now.Mode().IsRegular() {
		return Staged{}, failf(ReasonUnreadable, "%s changed while it was collected", f.rel)
	}
	limit, reason := streamBudget(c.Policy, c.Usage, streamed)
	sr := newScanReader(file, c.Scanner, f.rel, limit)
	sr.reason = reason
	digest, size, err := c.Blobs.Put(ctx, realm, sr)
	if err != nil {
		return Staged{}, err
	}
	st := Staged{Decl: d, Name: name, Rel: f.rel, SHA256: digest, Size: size}
	if d.Kind == KindImage {
		rc, err := c.Blobs.Open(realm, digest)
		if err != nil {
			return Staged{}, failf(ReasonMissingBlob, "%s: %v", f.rel, err)
		}
		defer func() { _ = rc.Close() }()
		if st.Image, err = ReadImageLayout(rc); err != nil {
			return Staged{}, err
		}
		st.Image.Ref = d.ImageRef
		if c.Scanner != nil { // decoded JSON text can hide a value from the raw-byte scan
			for _, m := range st.Image.Manifests {
				if len(c.Scanner.Scan([]byte(m.Platform))) != 0 || len(c.Scanner.Scan([]byte(m.Digest))) != 0 {
					return Staged{}, failf(ReasonSecretMatch, "an image field holds a registered secret value; nothing is staged")
				}
			}
		}
	}
	return st, nil
}

// streamBudget is how many bytes one file may stream: the per-file cap, or the
// room left in the store when that is smaller (then the refusal is size_cap).
func streamBudget(p Policy, u Usage, streamed int64) (int64, string) {
	if room := p.MaxTotal - u.Total - streamed; room < p.MaxFile {
		return max(room, 0), ReasonSizeCap
	}
	return p.MaxFile, ReasonFileCap
}
