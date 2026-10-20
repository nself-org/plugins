package tart

import (
	"context"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"net/url"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

// fakeSecrets is a per-attempt secret source returning fixed values.
type fakeSecrets map[string]string

func (f fakeSecrets) Resolve(context.Context, []string) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range f {
		out[k] = v
	}
	return out, nil
}

type netFacts = netscope.Facts

func modelScope(s string) model.NetworkScope { return model.NetworkScope(s) }

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func isolationAttemptFor(id, coordinator string) isolation.Attempt {
	return isolation.Attempt{ID: id, CoordinatorID: coordinator}
}
