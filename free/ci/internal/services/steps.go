package services

// Purpose: the small steps Up is made of: validating the requested specs,
// the Docker and CLI preflight (E637, E638), running one nself command, and
// checking component names against the CLI's own list (E639).
// Inputs:  specs, a Runner. Outputs: coded errors.
// Constraints: an nself failure message names the command and the tail of its
// (already redacted) output, never an argument value.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// versionTag is what a version may look like: an image tag, nothing a .env line could carry.
var versionTag = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func itoa(n int) string { return strconv.Itoa(n) }

// resolve turns specs into connection rows, refusing a bad or duplicate name.
func resolve(specs []Spec) ([]conn, error) {
	if len(specs) == 0 {
		return nil, fail(CodeUnknown, "no service was named")
	}
	seen, comps := map[string]bool{}, map[string]string{}
	out := make([]conn, 0, len(specs))
	for _, s := range specs {
		s.Name = strings.TrimSpace(s.Name)
		k := envKey(s.Name)
		if k == "" {
			return nil, fail(CodeUnknown, "service name %q has no letters or digits", s.Name)
		}
		if seen[k] {
			return nil, fail(CodeUnknown, "service %q is named twice", s.Name)
		}
		seen[k] = true
		s.Component = strings.ToLower(strings.TrimSpace(orDefault(s.Component, s.Name)))
		if s.Version != "" && !versionTag.MatchString(s.Version) {
			return nil, fail(CodeUnknown, "service %q has version %q, which is not an image tag", s.Name, s.Version)
		}
		comp, ok := components[s.Component]
		if !ok {
			return nil, fail(CodeUnknown, "service %q uses component %q, which is not a supported CI service dependency (supported: %s)", s.Name, s.Component, strings.Join(supported(), ", "))
		}
		if other, dup := comps[s.Component]; dup {
			return nil, fail(CodeUnknown, "services %q and %q both use component %q: one throwaway project runs one instance of a component", other, s.Name, s.Component)
		}
		comps[s.Component] = s.Name
		out = append(out, conn{Spec: s, Comp: comp})
	}
	return out, nil
}

// preflight checks Docker and the CLI commands before anything is created.
func (p *Provisioner) preflight(ctx context.Context) error {
	res, err := p.call(ctx, queryTimeout, Call{Argv: []string{p.docker(), "info"}})
	if err != nil || res.ExitCode != 0 {
		return fail(CodeNoDocker, "Docker is not available (%s info failed): start Docker or Colima, or run a job without services", p.docker())
	}
	for _, verb := range []string{"", "list", "enable", "start"} {
		argv := []string{p.nself(), "config", "services"}
		if verb != "" {
			argv = append(argv, verb)
		}
		res, err = p.call(ctx, queryTimeout, Call{Argv: append(argv, "--help"), Env: cliEnv})
		if err != nil || res.ExitCode != 0 {
			return fail(CodeNoCLI, "`%s` is not available: run `nself update`", strings.Join(argv[1:], " "))
		}
	}
	return nil
}

// owns proves that the project directory is the one this Handle created: the
// CLI resolves the project from the working directory (and may look at parent
// directories), so it must never run where our own .env is not.
func (h *Handle) owns() error {
	b, err := os.ReadFile(filepath.Join(h.Dir, ".env"))
	if err != nil {
		return fail(CodeProvision, "refusing to run nself in %s: no project .env there", h.Dir)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "PROJECT_NAME="+h.Project {
			return nil
		}
	}
	return fail(CodeProvision, "refusing to run nself in %s: its .env does not name project %s", h.Dir, h.Project)
}

// call runs c with a deadline so a stuck image pull cannot hang a job.
func (p *Provisioner) call(ctx context.Context, d time.Duration, c Call) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return p.Run.Run(ctx, c)
}

// Deadlines of one call: a docker query, and an nself command (which may pull images).
const (
	queryTimeout = 2 * time.Minute
	cliTimeout   = 15 * time.Minute
)

// cli runs `nself <args>` in the project directory of h, after proving the
// directory is ours (every command but `init`).
func (p *Provisioner) cli(ctx context.Context, h *Handle, args ...string) error {
	if args[0] != "init" {
		if err := h.owns(); err != nil {
			return err
		}
	}
	res, err := p.call(ctx, cliTimeout, Call{Dir: h.Dir, Argv: append([]string{p.nself()}, args...), Env: cliEnv, Mask: h.creds})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail(CodeProvision, "%s could not run: %v", p.describe(args), err)
	}
	if res.ExitCode != 0 {
		e := fail(CodeProvision, "%s failed (exit %d): %s", p.describe(args), res.ExitCode, tailOf(maskAll(res.Output, h.creds), 400))
		e.portTaken = portTakenOutput(res.Output) // the whole output: the tail may have cut the phrase
		return e
	}
	return nil
}

// describe names a command without its argument values.
func (p *Provisioner) describe(args []string) string {
	n := len(args)
	if n > 3 {
		n = 3
	}
	if len(args) > 0 && args[0] == "config" && len(args) > 2 && args[1] == "set" {
		n = 3 // `config set KEY`, never the value
	}
	return "nself " + strings.Join(args[:n], " ")
}

func tailOf(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = "..." + s[len(s)-n:]
	}
	return s
}

// checkNames asks the CLI for its optional-component list and refuses an
// unknown one. Required components (postgres) are part of every stack and are
// not in that list, so a request for only those makes no call.
func (p *Provisioner) checkNames(ctx context.Context, h *Handle, conns []conn) error {
	optional := false
	for _, c := range conns {
		optional = optional || !c.Comp.Required
	}
	if !optional {
		return nil
	}
	res, err := p.call(ctx, queryTimeout, Call{Dir: h.Dir, Argv: []string{p.nself(), "config", "services", "list", "--json"}, Env: cliEnv})
	if err != nil || res.ExitCode != 0 {
		return fail(CodeNoCLI, "`%s config services list --json` failed: run `nself update`", p.nself())
	}
	known := ComponentNames(res.Output)
	if len(known) == 0 {
		return fail(CodeNoCLI, "`%s config services list --json` returned no components: run `nself update`", p.nself())
	}
	for _, c := range conns {
		if !c.Comp.Required && !contains(known, c.Comp.Name) {
			return fail(CodeUnknown, "service %q: the CLI has no component %q (known: %s)", c.Spec.Name, c.Comp.Name, strings.Join(known, ", "))
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ComponentNames reads component names out of `config services list --json`
// output: the first JSON value in it (log lines may precede it), whatever
// envelope wraps the rows, collecting each row's "service" or "name".
func ComponentNames(out string) []string {
	for len(out) > 0 {
		if out[0] == '[' || out[0] == '{' {
			break
		}
		i := strings.IndexByte(out, '\n')
		if i < 0 {
			return nil
		}
		out = out[i+1:]
	}
	var doc any
	if json.NewDecoder(strings.NewReader(out)).Decode(&doc) != nil {
		return nil
	}
	set := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for k, e := range t {
				if s, ok := e.(string); ok && (k == "service" || k == "name") {
					set[s] = true
					continue
				}
				walk(e)
			}
		}
	}
	walk(doc)
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
