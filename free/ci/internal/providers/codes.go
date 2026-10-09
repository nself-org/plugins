package providers

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	for _, c := range []model.Code{
		{ID: "E700", Class: "config", Summary: "provider not configured or credential missing", Fix: "Configure the provider and credential"},
		{ID: "E701", Class: "auth", Summary: "credential lacks scope or permission", Fix: "Grant the named permission"},
		{ID: "E702", Class: "config", Summary: "provider rejected a misconfigured request", Fix: "Correct the repository, workflow, ref or project"},
		{ID: "E703", Class: "quota", Summary: "provider rate limited or quota exhausted", Fix: "Retry after the indicated time"},
		{ID: "E704", Class: "transient", Summary: "provider unavailable", Fix: "Retry after recovery"},
		{ID: "E705", Class: "config", Summary: "hosted evidence rejected", Fix: "Inspect evidence binding"},
		{ID: "E706", Class: "auth", Summary: "trigger rejected", Fix: "Check signature, token, timestamp and source"},
		{ID: "E707", Class: "config", Summary: "runner registration or teardown failed", Fix: "Inspect runner registration"},
		{ID: "E708", Class: "config", Summary: "job cannot be dispatched to provider", Fix: "Reduce or change the job request"},
		{ID: "E709", Class: "config", Summary: "usage import unavailable", Fix: "Check billing access and endpoint"},
	} {
		model.Register(c)
	}
}
