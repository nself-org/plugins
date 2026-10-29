package labels

// Purpose: the project authorization edit of `nself ci nodes projects add|remove|list` (P7-NODE-30):
//          change lifecycle.authorized_projects through the registry, one audit row per change.
// Inputs:  Env, a node id and project arguments: a repo id, or a directory (a path starting with `.`, `/`
//          or `~`, or containing `/`) that resolves to its repo id the way evidence names it.
// Outputs: the projects added or removed, or the node's authorized projects.
// Constraints: `--all` on add expands at call time to the explicit projects the PERSONAL store has seen
//          (no wildcard entry is ever stored) and is refused on a TEAM coordinator (team_wildcard);
//          removal keeps running attempts (the lease gate refuses new leases, E657); a project already
//          in the requested state is skipped, not an error.

import (
	"context"
	"regexp"
	"slices"
	"strings"
)

var projectIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:_.-]{0,127}$`)

// Projects returns the node's authorized projects.
func (e Env) Projects(ctx context.Context, node string) ([]string, error) {
	n, err := e.Registry.Get(ctx, node)
	if err != nil {
		return nil, err
	}
	return slices.Clone(n.Capability.Lifecycle.AuthorizedProjects), nil
}

// Resolve turns project arguments into repo ids; none given means the current directory.
func (e Env) Resolve(ctx context.Context, args []string) ([]string, error) {
	if len(args) == 0 {
		args = []string{"."}
	}
	var out []string
	for _, a := range args {
		id := a
		if isDir(a) {
			if e.Team || e.ResolveDir == nil {
				return nil, newErr(ReasonProjectUnknown, "%q is a directory: a coordinator takes repo ids only", a)
			}
			var ok bool
			if id, ok = e.ResolveDir(ctx, a); !ok || id == "" {
				return nil, newErr(ReasonProjectUnknown, "%q has no repository id (no git commit yet?)", a)
			}
		} else if !projectIDRE.MatchString(a) {
			return nil, newErr(ReasonProjectUnknown, "%q is not a repo id", a)
		}
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

func isDir(a string) bool {
	return strings.ContainsAny(a[:min(1, len(a))], "./~") || strings.Contains(a, "/")
}

// AddProjects authorizes the projects on node (all: every project the store has seen) and returns those newly added.
func (e Env) AddProjects(ctx context.Context, node string, args []string, all bool) ([]string, error) {
	var want []string
	var err error
	if all {
		if e.Team {
			return nil, newErr(ReasonTeamWildcard, "--all grants every project and is refused on a TEAM coordinator: name each project")
		}
		if want, err = e.Store.PipelineProjects(ctx); err != nil {
			return nil, err
		}
	} else if want, err = e.Resolve(ctx, args); err != nil {
		return nil, err
	}
	have, err := e.Projects(ctx, node)
	if err != nil {
		return nil, err
	}
	var added []string
	for _, p := range want {
		if slices.Contains(have, p) {
			continue
		}
		if _, err = e.Registry.Authorize(ctx, node, p, e.Actor); err != nil {
			return added, err
		}
		added = append(added, p)
	}
	return added, nil
}

// RemoveProjects deauthorizes the projects on node (all: every authorized project) and returns those removed.
func (e Env) RemoveProjects(ctx context.Context, node string, args []string, all bool) ([]string, error) {
	have, err := e.Projects(ctx, node)
	if err != nil {
		return nil, err
	}
	want := have
	if !all {
		if want, err = e.Resolve(ctx, args); err != nil {
			return nil, err
		}
	}
	var removed []string
	for _, p := range want {
		if !slices.Contains(have, p) {
			continue
		}
		if _, err = e.Registry.Deauthorize(ctx, node, p, e.Actor); err != nil {
			return removed, err
		}
		removed = append(removed, p)
	}
	return removed, nil
}
