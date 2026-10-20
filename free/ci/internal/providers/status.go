package providers

// Purpose: status reporting as a provider capability (P7-HOST-02, Epic D7).
// StatusReporter is the one Reporter interface of internal/report, under the
// name provider adapters use; a provider registers its adapter with
// report.RegisterReporter(kind, match, adapter) from init(). This file also
// registers the policy key providers.report_owner (personal|team) and reads it.
// Inputs:  an effective policy.
// Outputs: the reporting owner of a project.
// Constraints: no second reporter interface and no second status
// implementation; the key is operator scope only (Spend), so a repository
// cannot take over reporting; an unknown value is refused at decode and a
// missing value is personal.

import (
	"encoding/json"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// StatusReporter delivers a commit status to a code host. It is
// report.Reporter itself, not a copy.
type StatusReporter = report.Reporter

// KeyReportOwner is the policy key that names the reporting owner of a project.
const KeyReportOwner = "providers.report_owner"

var statusRegistrationErrors []error

// StatusRegistrationErrors returns what init() could not register; empty when
// the key is in the process-wide key set.
func StatusRegistrationErrors() []error { return append([]error(nil), statusRegistrationErrors...) }

func init() {
	if err := policy.RegisterKey(ReportOwnerKey()); err != nil {
		statusRegistrationErrors = append(statusRegistrationErrors, err)
	}
}

// ReportOwnerKey is the spec of providers.report_owner: operator scope only.
// It has NO registered default on purpose: the merge seeds a Fixed key with its
// default and then refuses any scope whose value differs (E670), so a default of
// "personal" would make "team" impossible to set and would change every
// effective-policy digest. A missing value reads as personal (ReportOwner).
func ReportOwnerKey() policy.KeySpec {
	return policy.KeySpec{Key: KeyReportOwner, Owner: "providers", Kind: policy.TightenFixed, Spend: true, Decode: decodeReportOwner}
}

func decodeReportOwner(raw json.RawMessage) (any, error) {
	var v string
	if err := json.Unmarshal(raw, &v); err != nil || (v != report.OwnerPersonal && v != report.OwnerTeam) {
		return nil, fmt.Errorf("want %q or %q", report.OwnerPersonal, report.OwnerTeam)
	}
	return v, nil
}

// ReportOwner is the reporting owner in force: report.OwnerTeam or
// report.OwnerPersonal (a missing or unexpected value is personal).
func ReportOwner(eff policy.Effective) string {
	if v, _, ok := eff.Value(KeyReportOwner); ok {
		if s, _ := v.(string); s == report.OwnerTeam {
			return report.OwnerTeam
		}
	}
	return report.OwnerPersonal
}
