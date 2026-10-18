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
	"sort"
	"strconv"
	"strings"
)

func itoa(n int) string { return strconv.Itoa(n) }

// resolve turns specs into connection rows, refusing a bad or duplicate name.
func resolve(specs []Spec) ([]conn, error) {
	if len(specs) == 0 {
		return nil, fail(CodeUnknown, "no service was named")
	}
	seen := map[string]bool{}
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
		comp, ok := components[s.Component]
		if !ok {
			return nil, fail(CodeUnknown, "service %q uses component %q, which is not a supported CI service dependency (supported: %s)", s.Name, s.Component, strings.Join(supported(), ", "))
		}
		out = append(out, conn{Spec: s, Comp: comp})
	}
	return out, nil
}

// preflight checks Docker and the CLI commands before anything is created.
func (p *Provisioner) preflight(ctx context.Context) error {
	res, err := p.Run.Run(ctx, Call{Argv: []string{p.docker(), "info"}})
	if err != nil || res.ExitCode != 0 {
		return fail(CodeNoDocker, "Docker is not available (%s info failed): start Docker or Colima, or run a job without services", p.docker())
	}
	res, err = p.Run.Run(ctx, Call{Argv: []string{p.nself(), "config", "services", "--help"}, Env: cliEnv})
	if err != nil || res.ExitCode != 0 {
		return fail(CodeNoCLI, "`%s config services` is not available: run `nself update`", p.nself())
	}
	return nil
}

// cli runs `nself <args>` in the project directory of h.
func (p *Provisioner) cli(ctx context.Context, h *Handle, args ...string) error {
	res, err := p.Run.Run(ctx, Call{Dir: h.Dir, Argv: append([]string{p.nself()}, args...), Env: cliEnv, Mask: h.creds})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fail(CodeProvision, "%s could not run: %v", p.describe(args), err)
	}
	if res.ExitCode != 0 {
		return fail(CodeProvision, "%s failed (exit %d): %s", p.describe(args), res.ExitCode, tailOf(maskAll(res.Output, h.creds), 400))
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
	res, err := p.Run.Run(ctx, Call{Dir: h.Dir, Argv: []string{p.nself(), "config", "services", "list", "--json"}, Env: cliEnv})
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
