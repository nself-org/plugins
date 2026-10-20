package runtime

// Purpose: turn what a provider collected into the evidence document of the attempt (Epic D5):
// the provider's own document, marked producer self-reported (the provider ran the job, the
// coordinator did not see it), signature coordinator-recorded (the coordinator is the one recording
// it) and carrying the provider_attestation. The run package's evidence check then holds the
// document to the delivery (schema, attempt id, revision).
// Inputs:  providers.Collected, the dispatch handle and the lease.
// Outputs: the evidence JSON for protocol.Result, or E705.
// Constraints: the attestation must name the provider, the run and the nonce of the handle this
//          coordinator dispatched; a document for another attempt or revision is refused here, so
//          evidence from a foreign run can never be recorded under this lease. Errors carry the
//          reason only, never the document.

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

func rejected(reason string) error {
	return providers.NewError(providers.Config, "E705", reason, 0, time.Time{})
}

func bindEvidence(c providers.Collected, h providers.Handle, l protocol.Lease, now time.Time) (json.RawMessage, error) {
	var ev model.Evidence
	if err := json.Unmarshal(c.Evidence, &ev); err != nil || ev.Schema != "ci.evidence/v1" {
		return nil, rejected("provider evidence is not a ci.evidence/v1 document")
	}
	if ev.Attempt == nil || ev.Attempt.ID != l.AttemptID {
		return nil, rejected("provider evidence names another attempt")
	}
	if !strings.EqualFold(ev.Revision, l.Checkout.Revision) {
		return nil, rejected("provider evidence names another revision")
	}
	att := c.Attestation
	if att.Provider != h.Provider || att.RunID != h.RunID || att.Nonce != h.Nonce {
		return nil, rejected("provider attestation does not match the dispatched run")
	}
	if att.VerifiedAt == "" {
		att.VerifiedAt = now.UTC().Format(time.RFC3339)
	}
	ev.Producer = model.Producer{Kind: "self-reported", Version: ev.Producer.Version}
	ev.Signature = model.Signature{Kind: "coordinator-recorded"}
	ev.ProviderAttestation = &att
	out, err := json.Marshal(ev)
	if err != nil {
		return nil, rejected("provider evidence cannot be recorded")
	}
	return out, nil
}
