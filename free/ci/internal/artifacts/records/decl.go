package records

// Purpose: turn the artifacts: block of a job (and the implicit outputs other
// Tickets register) into normalized declarations, once, at attempt start.
// Inputs:  model.Job, whether the job is release-kind, ImplicitFunc registrations.
// Outputs: []Decl, RegisterImplicit, RetentionDuration.
// Constraints: pure; every path and name goes through authz.ArtifactPath, so a
// traversal or absolute declaration is refused before the job runs (E683);
// an unknown kind, visibility or retention is E712 declaration; a release-kind
// job keeps its artifacts unless it sets a retention.

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// DefaultRetention is the retention of a job that sets none.
const DefaultRetention = "30d"

// ImplicitFunc adds declarations for a job (P7-CACHE-08 adds SBOM outputs).
type ImplicitFunc func(job model.Job) []model.ArtifactDecl

var (
	implicitMu sync.Mutex
	implicit   []ImplicitFunc
)

// RegisterImplicit adds a source of implicit declarations; call from init().
func RegisterImplicit(f ImplicitFunc) {
	implicitMu.Lock()
	defer implicitMu.Unlock()
	implicit = append(implicit, f)
}

// ImplicitDecls collects every registered implicit declaration for job.
func ImplicitDecls(job model.Job) []model.ArtifactDecl {
	implicitMu.Lock()
	fs := append([]ImplicitFunc(nil), implicit...)
	implicitMu.Unlock()
	var out []model.ArtifactDecl
	for _, f := range fs {
		out = append(out, f(job)...)
	}
	return out
}

// RetentionDuration parses "keep" (keep=true) or a duration such as 30d, 2w,
// 12h, 90m. Zero and negative durations are refused.
func RetentionDuration(s string) (d time.Duration, keep bool, err error) {
	if s == KeepRetention {
		return 0, true, nil
	}
	unit := map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}
	if len(s) < 2 || unit[s[len(s)-1]] == 0 {
		return 0, false, failf(ReasonDeclaration, "retention %q is not keep or a duration like 30d", s)
	}
	n, e := strconv.Atoi(s[:len(s)-1])
	if e != nil || n <= 0 || n > 36500 {
		return 0, false, failf(ReasonDeclaration, "retention %q is not keep or a duration like 30d", s)
	}
	return time.Duration(n) * unit[s[len(s)-1]], false, nil
}

// Normalize validates job.Artifacts plus extra and fills the defaults.
func Normalize(job model.Job, release bool, extra []model.ArtifactDecl) ([]Decl, error) {
	all := append(append([]model.ArtifactDecl(nil), job.Artifacts...), extra...)
	out := make([]Decl, 0, len(all))
	for i, d := range all {
		path, err := cleanName(strings.TrimRight(d.Path, "/"))
		if err != nil {
			return nil, err
		}
		name := ""
		if d.Name != "" {
			if name, err = cleanName(d.Name); err != nil {
				return nil, err
			}
		}
		kind, ok := ParseKind(string(d.Kind))
		if !ok {
			return nil, failf(ReasonDeclaration, "artifact %q: unknown kind %q", d.Path, d.Kind)
		}
		if d.Image != nil && kind != KindImage {
			return nil, failf(ReasonDeclaration, "artifact %q: image: needs kind image", d.Path)
		}
		vis := authz.Visibility(d.Visibility)
		if vis == "" {
			vis = authz.VisibilityProject
		}
		if vis != authz.VisibilityProject && vis != authz.VisibilityRestricted {
			return nil, failf(ReasonDeclaration, "artifact %q: unknown visibility %q", d.Path, d.Visibility)
		}
		ret := d.Retention
		switch {
		case ret == "" && release:
			ret = KeepRetention
		case ret == "":
			ret = DefaultRetention
		}
		if _, _, err = RetentionDuration(ret); err != nil {
			return nil, err
		}
		decl := Decl{Ord: i, Path: path, Name: name, Kind: kind, Retention: ret, Visibility: vis}
		if d.Image != nil {
			decl.ImageRef = d.Image.Ref
		}
		out = append(out, decl)
	}
	return out, nil
}

// cleanName runs a declared path or name through the shared sanitiser.
func cleanName(s string) (string, error) {
	clean, err := authz.ArtifactPath(authz.Entry{Name: s, Mode: 0}, authz.Limits{})
	if err != nil {
		return "", err
	}
	if clean == "." {
		return "", failf(ReasonDeclaration, "artifact path %q names the workdir itself", s)
	}
	return clean, nil
}
