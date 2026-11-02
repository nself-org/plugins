package run

import (
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// teamFacts describes submitted work without borrowing the operator's personal
// ownership or dedicated-runner exception. The coordinator passes refs only.
func teamFacts(a AttemptInfo) secrets.Facts {
	iso := a.Job.Isolation
	if iso == "" {
		iso = "process"
	}
	return secrets.Facts{
		Job: trust.JobFacts{Trust: a.SourceClass, Mode: trust.Team,
			Release: a.Job.Kind == "release" && a.ReleaseAuthorized},
		Runner:  trust.RunnerFacts{Ownership: "operator", Isolation: iso},
		Allowed: a.SecretClasses, HasPolicy: a.HasSecretClasses,
	}
}
