package coordinator

// Purpose: the ingress registry (P7-NODE-10, HOST delta 5, review R1): a provider adapter registers an
// Ingress; the coordinator hands it only the requests it matches, caps the body before Accept, and
// enqueues the Delivery it returns. P7-HOST-01 registers the hosted providers here.
// Inputs:  Ingress implementations registered at init; HTTP requests on /webhook and POST /.
// Outputs: Delivery values for the intake; 413 for an oversize body, 403 when Accept refuses.
// Constraints: a request an Ingress matched is verified only by it (the v1.4 shared secret does not run
//          for it); an Ingress that checks per-repo secrets implements SecretHolder, which lets the
//          coordinator start without --secret; Accept errors are logged by name only, never echoed.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Delivery is one trigger from any source.
type Delivery struct {
	Source       string          // provider or origin: "github", "schedule", "manual", ...
	DeliveryID   string          // the provider's delivery id; "" when the source has none
	Project      string          // the base repository, e.g. "owner/repo"
	Revision     string          // the commit the trigger names
	TriggerClass string          // "change" for push, pull request and manual runs; providers add their own
	Facts        json.RawMessage // source facts (event, ref, clone_url, ...); opaque to the intake
	Pipeline     string          // the requested pipeline, "" for the default
	Synthetic    *model.Pipeline // a pipeline the source built itself (runner demand), if any

	// Prepare synthesizes a delivery only after repository authorization.
	Prepare func(context.Context, *Delivery) error `json:"-"`
}

// Ingress turns the requests of one provider into Deliveries.
type Ingress interface {
	// Name is unique among registered ingresses.
	Name() string
	// Match reports whether r belongs to this ingress.
	Match(r *http.Request) bool
	// Accept verifies and parses a matched request; body is the whole body (at most 25 MiB). An error
	// refuses the request with 403 and enqueues nothing, except an error that has a
	// DeclineReason() string method (P7-HOST-16): the request was fine but must not enqueue,
	// and the answer is 200 {"accepted": false, "reason": ...}.
	Accept(ctx context.Context, r *http.Request, body []byte) (Delivery, error)
}

// SecretHolder may be implemented by an Ingress that verifies deliveries with its own secrets. When one
// reports true, `ci serve` starts without --secret or GITHUB_WEBHOOK_SECRET.
type SecretHolder interface{ HasSecrets() bool }

var ingresses struct {
	sync.Mutex
	list []Ingress
}

// RegisterIngress adds an Ingress. A nil value, an empty name or a repeated name is refused.
func RegisterIngress(i Ingress) error {
	if i == nil || strings.TrimSpace(i.Name()) == "" {
		return newErr("", ReasonInvalidConfig, "ingress needs a name")
	}
	ingresses.Lock()
	defer ingresses.Unlock()
	for _, have := range ingresses.list {
		if have.Name() == i.Name() {
			return newErr("", ReasonDuplicateName, "ingress %q is already registered", i.Name())
		}
	}
	ingresses.list = append(ingresses.list, i)
	return nil
}

func registeredIngresses() []Ingress {
	ingresses.Lock()
	defer ingresses.Unlock()
	return append([]Ingress(nil), ingresses.list...)
}

// anySecrets reports whether a registered ingress holds secrets of its own.
func anySecrets(list []Ingress) bool {
	for _, i := range list {
		if h, ok := i.(SecretHolder); ok && h.HasSecrets() {
			return true
		}
	}
	return false
}
