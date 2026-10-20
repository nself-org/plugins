package cloudvm

// Purpose: Teardown and List of the cloud VM provider (Epic D13): delete one server by id and list
// servers by label, both through `nself infra server` with the token in the child's environment only.
// Inputs:  a server id, or a label set (every key and value a plain label token).
// Outputs: nil / the matching machines, or a classified error (burst.ErrUnavailable and the like).
// Constraints: Teardown is synchronous (`destroy --id N --force-no-backup --release-ip --json` returns
// when the server is gone) and idempotent: a not-found answer, and an id that is not a number (no
// server can have it), are success; List builds `--label-selector k=v,k=v` from labels that pass the
// label shape, requires at least one label (never a whole-project listing) and filters the answer by
// every requested label again, so a CLI that ignored the selector cannot widen the result; the
// offer of the intent is withdrawn once the machine is gone, which also revokes a certificate issued to it.

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

var idShape = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)

// Teardown implements burst.Provider.
func (p *Provider) Teardown(ctx context.Context, id string) error {
	if !idShape.MatchString(id) {
		return nil // no server of the provider has this id: nothing to delete
	}
	pol, err := p.cfg.Spend(ctx)
	if err != nil {
		return fail(ReasonRecipe, err, "the spend policy could not be read")
	}
	if pol.Burst.CloudVM.TokenFile == "" {
		return fail(ReasonRecipe, nil, "%s is not set", "burst.cloudvm.token_file")
	}
	tok, err := readTokenFile(pol.Burst.CloudVM.TokenFile)
	if err != nil {
		return err
	}
	if err = p.destroy(ctx, tok, id); err != nil {
		return err
	}
	p.mu.Lock()
	token := p.byID[id]
	delete(p.byID, id)
	p.mu.Unlock()
	if token != "" {
		p.withdraw(ctx, token)
	}
	return nil
}

// selector renders labels as a Hetzner label selector, refusing anything but plain label tokens.
func selector(labels map[string]string) (string, error) {
	if len(labels) == 0 {
		return "", fail(ReasonRecipe, nil, "a list needs at least one label")
	}
	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		if !labelShape.MatchString(k) || !labelShape.MatchString(v) {
			return "", fail(ReasonRecipe, nil, "label %q has a form the selector cannot carry", k)
		}
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ","), nil
}

// List implements burst.Provider.
func (p *Provider) List(ctx context.Context, labels map[string]string) ([]burst.Resource, error) {
	if _, err := selector(labels); err != nil {
		return nil, err
	}
	pol, err := p.cfg.Spend(ctx)
	if err != nil {
		return nil, fail(ReasonRecipe, err, "the spend policy could not be read")
	}
	if pol.Burst.CloudVM.TokenFile == "" {
		return nil, fail(ReasonRecipe, nil, "%s is not set", "burst.cloudvm.token_file")
	}
	tok, err := readTokenFile(pol.Burst.CloudVM.TokenFile)
	if err != nil {
		return nil, err
	}
	return p.list(ctx, tok, labels)
}

// list runs `server list` for the labels.
func (p *Provider) list(ctx context.Context, tok string, labels map[string]string) ([]burst.Resource, error) {
	sel, err := selector(labels)
	if err != nil {
		return nil, err
	}
	data, err := p.run(ctx, tok, "list", "--label-selector", sel)
	if err != nil {
		return nil, err
	}
	var ans struct {
		Servers []wireServer `json:"servers"`
	}
	if jerr := json.Unmarshal(data, &ans); jerr != nil {
		return nil, fail(ReasonAnswer, nil, "the list answer is not a server list")
	}
	var out []burst.Resource
	for _, w := range ans.Servers {
		if w.ID <= 0 {
			return nil, fail(ReasonAnswer, nil, "the list answer holds a server without an id")
		}
		if r := w.resource(p.cfg.Now()); burst.Matches(r.Labels, labels) {
			out = append(out, r)
		}
	}
	return out, nil
}
