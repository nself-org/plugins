package gates

import (
	"errors"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Registry accepts built-in and extension check definitions.
type Registry struct{ defs map[string]CheckDef }

func NewRegistry() *Registry { return &Registry{defs: map[string]CheckDef{}} }
func (r *Registry) Register(d CheckDef) error {
	if d.ID == "" || d.Kind == "" || d.Tool == "" || len(d.Argv) == 0 || len(d.VersionProbe) == 0 || d.Remediation == "" || (d.Path != "fast" && d.Path != "deep") {
		return errors.New("incomplete check definition")
	}
	if r.defs == nil {
		r.defs = map[string]CheckDef{}
	}
	if _, ok := r.defs[d.ID]; ok {
		return ErrDuplicate
	}
	r.defs[d.ID] = d
	return nil
}
func (r *Registry) Has(id string) bool             { _, ok := r.Get(id); return ok }
func (r *Registry) Get(id string) (CheckDef, bool) { d, ok := r.defs[id]; return d, ok }
func (r *Registry) All() []CheckDef {
	out := make([]CheckDef, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// DefaultRegistry provides every detector binding and the local security set.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, d := range builtins() {
		_ = r.Register(d)
	}
	return r
}

func def(id, kind, tool string, argv []string, required, substantive bool, path model.JobPath, fix string) CheckDef {
	return CheckDef{ID: id, Kind: kind, Tool: tool, Argv: argv, VersionProbe: []string{tool, "--version"}, RequiredDefault: required, Substantive: substantive, Path: path, Remediation: fix}
}

func builtins() []CheckDef {
	rows := []CheckDef{
		def("go.fmt", "format", "gofmt", []string{"gofmt", "-l", "."}, true, true, "fast", "Install the Go toolchain"),
		def("go.vet", "lint", "go", []string{"go", "vet", "./..."}, true, true, "fast", "Install the Go toolchain"),
		def("go.test", "test", "go", []string{"go", "test", "-count=1", "./..."}, true, true, "fast", "Install the Go toolchain"),
		def("go.govulncheck", "security", "govulncheck", []string{"govulncheck", "-json", "./..."}, false, true, "deep", "Install govulncheck and refresh vulnerability data"),
		def("go.coverage", "coverage", "go", []string{"go", "test", "-coverprofile=coverage.out", "./..."}, false, true, "deep", "Install the Go toolchain"),
		def("node.lint", "lint", "npm", []string{"npm", "run", "lint"}, true, true, "fast", "Install the declared Node package manager"),
		def("node.typecheck", "lint", "npm", []string{"npm", "run", "typecheck"}, true, true, "fast", "Install the declared Node package manager"),
		def("node.test", "test", "npm", []string{"npm", "run", "test"}, true, true, "fast", "Install the declared Node package manager"),
		def("node.build", "build", "npm", []string{"npm", "run", "build"}, true, true, "deep", "Install the declared Node package manager"),
		def("node.coverage", "coverage", "npm", []string{"npm", "run", "test", "--", "--coverage"}, false, true, "deep", "Install the declared Node package manager"),
		def("rust.clippy", "lint", "cargo", []string{"cargo", "clippy", "--all-targets", "--all-features", "--", "--deny", "warnings"}, true, true, "fast", "Install Rust and clippy"),
		def("rust.test", "test", "cargo", []string{"cargo", "test", "--all-features"}, true, true, "fast", "Install Rust"),
		def("flutter.analyze", "lint", "flutter", []string{"flutter", "analyze"}, true, true, "fast", "Install Flutter"),
		def("flutter.test", "test", "flutter", []string{"flutter", "test", "--reporter", "compact"}, true, true, "fast", "Install Flutter"),
		def("secrets.gitleaks", "security", "gitleaks", []string{"gitleaks", "detect", "--source", ".", "--exit-code", "1"}, true, false, "fast", "Install gitleaks and put it on PATH"),
		def("security.trivy.fs", "security", "trivy", []string{"trivy", "fs", "--format", "json", "--exit-code", "1", "."}, false, true, "deep", "Install trivy and download its vulnerability DB"),
		def("container.trivy.config", "security", "trivy", []string{"trivy", "config", "--format", "json", "--exit-code", "1", "{target}"}, false, true, "deep", "Install trivy and download its vulnerability DB"),
		def("container.hadolint", "lint", "hadolint", []string{"hadolint", "--format", "json", "{target}"}, false, true, "deep", "Install hadolint"),
		def("container.docker", "input", "git", []string{"git", "status", "--porcelain", "--", "{target}"}, true, false, "fast", "Install git"),
		def("container.compose", "input", "git", []string{"git", "status", "--porcelain", "--", "{target}"}, true, false, "fast", "Install git"),
		def("inputs.lockfiles", "input", "git", []string{"git", "status", "--porcelain", "--", "{target}"}, true, false, "fast", "Install git"),
	}
	for i := range rows {
		if rows[i].ID == "go.fmt" {
			rows[i].VersionProbe = []string{"go", "version"}
		}
	}
	return rows
}
