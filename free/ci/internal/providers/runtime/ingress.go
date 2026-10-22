package runtime

// Purpose: the ingress adapter (Epic D4, NODE-10 registry). One coordinator.Ingress covers every
// registered providers.TriggerSource, including sources registered after this package's init: Match
// asks each source, Accept hands the matched request to that source's Verify and turns the verified
// TriggerFacts into a Delivery. For Kind runner_demand it calls the source's registered
// DemandHandler and sets Delivery.Synthetic.
// Inputs:  http requests the coordinator's intake routes to Match; the source registry.
// Outputs: coordinator.Delivery (Facts = the TriggerFacts JSON); *Declined for a demand the runtime
//          will not enqueue; the Verify error (the coordinator answers 403) for a bad request.
// Constraints: this package starts no server and opens no listener (NODE-10 owns intake). A request a
//          source matched is verified only by that source. A refusal's reason is a safe string:
//          provider error codes and our own text, never the handler's raw error. TriggerClass is
//          "change" for push, pull request, tag, manual and api triggers (the intake coalesces them
//          on one revision) and the kind itself for schedule and runner_demand. Dormant unless
//          compat.V15().

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// IngressName is the name the adapter registers under.
const IngressName = "providers"

// Declined is returned by Accept when a verified request is fine but must not enqueue anything (a
// runner demand with no handler, or one the handler refused). The coordinator answers it with 200
// {"accepted": false, "reason": ...} so the provider does not retry.
type Declined struct{ Reason, Project string }

func (d *Declined) DeclineProject() string { return d.Project }

func (d *Declined) Error() string         { return "delivery declined: " + d.Reason }
func (d *Declined) DeclineReason() string { return d.Reason }

// IngressAdapter is the coordinator.Ingress for every registered trigger source.
type IngressAdapter struct{}

func init() {
	if err := coordinator.RegisterIngress(IngressAdapter{}); err != nil {
		panic(err)
	}
}

// Name is "providers".
func (IngressAdapter) Name() string { return IngressName }

// Match reports whether a registered source claims the request.
func (IngressAdapter) Match(r *http.Request) bool {
	// compat.V15(P7-HOST-16): provider triggers go to the v1.4 shared-secret path -> provider sources verify their own deliveries
	if !compat.V15() {
		return false
	}
	return sourceFor(r) != nil
}

func sourceFor(r *http.Request) providers.TriggerSource {
	for _, s := range providers.Sources() {
		if s.Match(r) {
			return s
		}
	}
	return nil
}

// HasSecrets is true only when there is a registered source and every one of them holds per-repo
// secrets of its own and says so with its own HasSecrets. The coordinator applies the answer to every
// request this one ingress handles (it skips --secret and --allowed-repos), so a single source that
// does not say so keeps all of them under both.
func (IngressAdapter) HasSecrets() bool {
	if !compat.V15() {
		return false
	}
	sources := providers.Sources()
	for _, s := range sources {
		if h, ok := s.(interface{ HasSecrets() bool }); !ok || !h.HasSecrets() {
			return false
		}
	}
	return len(sources) > 0
}

// Accept verifies the request with the matching source and builds the Delivery.
func (IngressAdapter) Accept(ctx context.Context, r *http.Request, body []byte) (coordinator.Delivery, error) {
	src := sourceFor(r)
	if src == nil {
		return coordinator.Delivery{}, errors.New("no trigger source claims the request")
	}
	facts, err := src.Verify(ctx, r, body)
	if err != nil {
		return coordinator.Delivery{}, err
	}
	if err = facts.Validate(); err != nil {
		return coordinator.Delivery{}, err
	}
	if facts.Source == "" {
		facts.Source = src.Name()
	} else if facts.Source != src.Name() { // the delivery's identity is the source that verified it
		return coordinator.Delivery{}, errors.New("trigger facts name another source")
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return coordinator.Delivery{}, err
	}
	d := coordinator.Delivery{Source: facts.Source, DeliveryID: facts.DeliveryID, Project: facts.Repo, Revision: facts.Revision, TriggerClass: triggerClass(facts.Kind), Facts: raw}
	if facts.Kind != "runner_demand" {
		return d, nil
	}
	d.Prepare = func(ctx context.Context, d *coordinator.Delivery) error {
		handler := providers.DemandHandlerFor(src.Name())
		if handler == nil {
			return &Declined{Project: facts.Repo, Reason: "no demand handler is registered for " + src.Name()}
		}
		pipeline, err := handler.Synthesize(ctx, facts)
		if err != nil {
			return &Declined{Project: facts.Repo, Reason: declineReason(err)}
		}
		if pipeline == nil {
			return &Declined{Project: facts.Repo, Reason: "the demand handler declined the demand"}
		}
		d.Synthetic = pipeline
		return nil
	}

	return d, nil
}

func triggerClass(k model.TriggerKind) string {
	switch k {
	case "schedule", "runner_demand":
		return string(k)
	}
	return "change"
}

// declineReason is the part of a handler's error that is safe to send back to the provider.
func declineReason(err error) string {
	var pe *providers.Error
	if errors.As(err, &pe) && pe.Code != "" {
		return "the demand handler refused: " + pe.Code
	}
	return "the demand handler refused the demand"
}

var _ coordinator.Ingress = IngressAdapter{}
var _ coordinator.SecretHolder = IngressAdapter{}
