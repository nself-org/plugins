package onboard

// Reason is the closed list of refusal and failure reasons of contract
// infra.v1. Every refusal, failed phase or rollback names exactly one.
//
// Versioning: adding a Reason value is a contract minor (readers must accept
// an unknown value as a generic failure); removing or renaming one is a
// contract major. Keep AllReasons and the schema enum in step with this list.
type Reason string

// The reason values of contract infra.v1.
const (
	// ReasonStale means the price list or plan is older than its allowed age.
	ReasonStale Reason = "stale"
	// ReasonInvalid means an input or a plan failed validation.
	ReasonInvalid Reason = "invalid"
	// ReasonUnbound means a step ran without the approved plan hash it needs.
	ReasonUnbound Reason = "unbound"
	// ReasonExpired means an approval or credential is past its lifetime.
	ReasonExpired Reason = "expired"
	// ReasonScope means the credential lacks the access the step needs.
	ReasonScope Reason = "scope"
	// ReasonRegion means the location is unknown or does not offer the type.
	ReasonRegion Reason = "region"
	// ReasonQuota means the provider account limit would be exceeded.
	ReasonQuota Reason = "quota"
	// ReasonBudget means the quote exceeds the approved spending limit.
	ReasonBudget Reason = "budget"
	// ReasonOffline means the provider or the server could not be reached.
	ReasonOffline Reason = "offline"
	// ReasonUnsupported means the provider, image or option is not shipped.
	ReasonUnsupported Reason = "unsupported"
	// ReasonConflict means a resource with the same identity already exists.
	ReasonConflict Reason = "conflict"
	// ReasonLeftover means a resource survived a rollback or teardown.
	ReasonLeftover Reason = "leftover"
)

// AllReasons returns every Reason of contract infra.v1, in contract order.
func AllReasons() []Reason {
	return []Reason{
		ReasonStale, ReasonInvalid, ReasonUnbound, ReasonExpired,
		ReasonScope, ReasonRegion, ReasonQuota, ReasonBudget,
		ReasonOffline, ReasonUnsupported, ReasonConflict, ReasonLeftover,
	}
}

// Valid reports whether r is one of the contract reason values.
func (r Reason) Valid() bool {
	for _, v := range AllReasons() {
		if r == v {
			return true
		}
	}
	return false
}
