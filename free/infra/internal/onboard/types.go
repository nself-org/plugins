// Package onboard holds the frozen wire types of contract infra.v1: the
// server onboarding plan, its quote, the token report and the run state.
//
// Purpose: one audited place for the JSON shapes that onboarding reads and
// writes, matched field for field by schema/infra-v1.schema.json.
// Inputs: none (pure type definitions and enum validation).
// Outputs: none.
// Constraints: provider-neutral. Provider ids are strings, never int64.
// Money is a decimal string, never a float. No field of Plan or RunState
// carries a token, key or other secret; TokenReport only describes a token.
package onboard

import "time"

// ContractVersion is the value of the version field of every infra.v1 object.
const ContractVersion = "infra.v1"

// PlanHash is the content hash of a Plan, written "sha256:" plus 64 lowercase
// hex digits. Approval binds to this value.
type PlanHash string

// Phase is the closed list of onboarding run phases of contract infra.v1.
// Adding a value is a contract minor; removing or renaming one is a major.
type Phase string

// The phase values of contract infra.v1, in run order; the last four are
// the terminal and recovery states.
const (
	// PhasePreflight checks inputs, credentials and provider reachability.
	PhasePreflight Phase = "preflight"
	// PhaseQuoted means a priced plan exists and awaits approval.
	PhaseQuoted Phase = "quoted"
	// PhaseApproved means the owner approved the plan hash.
	PhaseApproved Phase = "approved"
	// PhaseProvisioning means the create request was sent to the provider.
	PhaseProvisioning Phase = "provisioning"
	// PhaseProvisioned means the server exists and is running.
	PhaseProvisioned Phase = "provisioned"
	// PhaseHardened means the hardening profile was applied and verified.
	PhaseHardened Phase = "hardened"
	// PhaseInstalled means the stack was installed on the server.
	PhaseInstalled Phase = "installed"
	// PhaseTLS means certificates were issued and served.
	PhaseTLS Phase = "tls"
	// PhaseHealthy means the health checks passed.
	PhaseHealthy Phase = "healthy"
	// PhaseDone means the run finished successfully.
	PhaseDone Phase = "done"
	// PhaseFailed means the run stopped; RunState.Reason says why.
	PhaseFailed Phase = "failed"
	// PhaseRolledBack means the resources of a failed run were removed.
	PhaseRolledBack Phase = "rolled_back"
	// PhaseTornDown means a finished server was removed on request.
	PhaseTornDown Phase = "torn_down"
)

// AllPhases returns every Phase of contract infra.v1, in contract order.
func AllPhases() []Phase {
	return []Phase{
		PhasePreflight, PhaseQuoted, PhaseApproved, PhaseProvisioning,
		PhaseProvisioned, PhaseHardened, PhaseInstalled, PhaseTLS,
		PhaseHealthy, PhaseDone, PhaseFailed, PhaseRolledBack, PhaseTornDown,
	}
}

// Valid reports whether p is one of the contract phase values.
func (p Phase) Valid() bool {
	for _, v := range AllPhases() {
		if p == v {
			return true
		}
	}
	return false
}

// Plan is the provider-neutral description of one server to onboard. Its
// hash (PlanHash) is what the owner approves.
type Plan struct {
	Version            string            `json:"version"`
	Provider           string            `json:"provider"`
	ServerType         string            `json:"server_type"`
	Location           string            `json:"location"`
	Image              string            `json:"image"`
	Name               string            `json:"name"`
	Labels             map[string]string `json:"labels"`
	HardeningDigest    string            `json:"hardening_digest"`
	QuoteTotal         string            `json:"quote_total"`
	Currency           string            `json:"currency"`
	PriceListFetchedAt time.Time         `json:"price_list_fetched_at"`
}

// Quote is the priced breakdown of a Plan from one provider price list.
type Quote struct {
	Version   string      `json:"version"`
	Provider  string      `json:"provider"`
	Location  string      `json:"location"`
	Currency  string      `json:"currency"`
	Lines     []PriceLine `json:"lines"`
	Total     string      `json:"total"`
	FetchedAt time.Time   `json:"fetched_at"`
}

// PriceLine is one priced item of a Quote. Amounts are decimal strings in
// the currency of the Quote.
type PriceLine struct {
	Item    string `json:"item"`
	Kind    string `json:"kind"`
	Hourly  string `json:"hourly"`
	Monthly string `json:"monthly"`
}

// TokenReport describes a provider credential without carrying it: where it
// came from, whether it works and what it may do. ExpiresAt is the expiry the
// provider reports, nil when the provider reports none.
type TokenReport struct {
	Version   string     `json:"version"`
	Provider  string     `json:"provider"`
	Present   bool       `json:"present"`
	Source    string     `json:"source"`
	Valid     bool       `json:"valid"`
	Scope     string     `json:"scope"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Reason    Reason     `json:"reason,omitempty"`
}

// RunState is the persisted progress of one onboarding run, bound to the
// approved PlanHash. Adopted is true when the run took over an existing
// server: rollback and teardown detach that server and never delete it.
type RunState struct {
	Version   string       `json:"version"`
	RunID     string       `json:"run_id"`
	PlanHash  PlanHash     `json:"plan_hash"`
	Phase     Phase        `json:"phase"`
	Reason    Reason       `json:"reason,omitempty"`
	Adopted   bool         `json:"adopted"`
	ServerID  string       `json:"server_id,omitempty"`
	IPv4      string       `json:"ipv4,omitempty"`
	IPv6      string       `json:"ipv6,omitempty"`
	History   []PhaseEvent `json:"history"`
	StartedAt time.Time    `json:"started_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// PhaseEvent records one phase transition of a run.
type PhaseEvent struct {
	Phase  Phase     `json:"phase"`
	At     time.Time `json:"at"`
	Reason Reason    `json:"reason,omitempty"`
}
