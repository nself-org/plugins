// Package providers defines the experimental v1 capacity provider contract.
// Core execution owns admission and trust; providers expose optional capabilities.
package providers

import (
	"context"
	"net/http"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type Provider interface {
	ID() string
	Describe(context.Context, Query) ([]model.Capability, error)
}
type Query struct{ Project, Repo, Visibility string }
type Attempt struct {
	ID                                                                      string
	N                                                                       int
	Project, Repo, Revision, FetchRef, SpecDigest, CompatMode, Trust, Label string
	Spec                                                                    []byte
	TimeoutMs                                                               int64
}
type Handle struct{ Provider, RunID, Nonce, Ref string }
type LiveState string

const (
	Queued   LiveState = "queued"
	Running  LiveState = "running"
	Finished LiveState = "finished"
	Vanished LiveState = "vanished"
)

type LeaseProfile struct{ TTL, Poll time.Duration }
type Collected struct {
	Evidence    []byte
	Attestation model.ProviderAttestation
}
type Dispatcher interface {
	Dispatch(context.Context, Attempt) (Handle, error)
	Liveness(context.Context, Handle) (LiveState, error)
	Cancel(context.Context, Handle) error
	Collect(context.Context, Handle) (Collected, error)
	Profile() LeaseProfile
}
type RunnerRequest struct {
	Attempt Attempt
	Scope   RunnerScope
	Labels  []string
	GroupID int64
}
type RunnerScope struct{ Project, Repo string }
type RunnerRef struct {
	ID    string
	Scope RunnerScope
}
type RunnerCredential struct {
	Kind, Value string
	ExpiresAt   time.Time
}
type Restriction struct {
	ProtectedOnly bool
	Workflows     []string
}
type RunnerRegistrar interface {
	Mint(context.Context, RunnerRequest) (RunnerCredential, error)
	Deregister(context.Context, RunnerRef) error
	Orphans(context.Context, RunnerScope, time.Time) ([]RunnerRef, error)
	Restriction(context.Context, RunnerScope, int64) (Restriction, error)
}
type TriggerSource interface {
	Name() string
	Match(*http.Request) bool
	Verify(context.Context, *http.Request, []byte) (model.TriggerFacts, error)
}
type TrustFacts interface {
	Refetch(context.Context, model.TriggerFacts) (model.TriggerFacts, error)
}
type UsageSource interface {
	Usage(context.Context, Period) ([]UsageRow, error)
}
type Period struct{ From, To time.Time }
type UsageRow struct {
	Account      string
	Date         time.Time
	Product, SKU string
	Quantity     float64
	Unit         string
	NetMicro     int64
	Repo         string
}
type Health struct {
	State, Reason  string
	Since, RetryAt time.Time
}
type HealthProbe interface{ Health(context.Context) Health }
