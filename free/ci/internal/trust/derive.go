package trust

import "github.com/nself-org/plugins/free/ci/internal/model"

type JobFacts struct {
	Trust          model.TrustClass
	SecretClasses  []model.SecretClass
	Network        model.NetworkScope
	Mode           CoordinatorMode
	PersonalOwnRun bool
	Release        bool
}
type RunnerFacts struct {
	Ownership       string
	Dedicated       bool
	Hosted          bool
	UIDSeparation   bool
	PersonalOwnNode bool
	NoLANRoute      bool
	Isolation       model.Isolation
}

func hasSecret(classes []model.SecretClass) bool {
	for _, c := range classes {
		if c != "none" {
			return true
		}
	}
	return false
}
func releaseSecret(classes []model.SecretClass) bool {
	for _, c := range classes {
		if c == "release" || c == "deploy" || c != "none" && c != "project" && c != "environment" && c != "team" {
			return true
		}
	}
	return false
}
func MinIsolation(job JobFacts, runner RunnerFacts) model.Isolation {
	trust := DecodeTrust(string(job.Trust))
	if trust == "untrusted" {
		if runner.Hosted {
			return "hosted-disposable"
		}
		return "sandboxed-container"
	}
	if releaseSecret(job.SecretClasses) && !runner.Dedicated {
		return "vm"
	}
	if trust == "collaborator" {
		return "container"
	}
	if runner.PersonalOwnNode && job.PersonalOwnRun && (trust == "owner" || trust == "internal") {
		return "process"
	}
	return "container"
}

// RetryIsolation keeps the previous attempt's floor during replacement.
func RetryIsolation(previous, required model.Isolation) model.Isolation {
	previous = DecodeIsolation(string(previous))
	required = DecodeIsolation(string(required))
	if RankIsolation(previous) >= RankIsolation(required) {
		return previous
	}
	return required
}

func DefaultNetwork(trust model.TrustClass) model.NetworkScope {
	switch DecodeTrust(string(trust)) {
	case "owner", "internal":
		return "lan"
	case "collaborator":
		return "internet"
	default:
		return "restricted"
	}
}
func SecretEligibility(job JobFacts, runner RunnerFacts) []Reason {
	var reasons []Reason
	if !hasSecret(job.SecretClasses) {
		return reasons
	}
	if DecodeTrust(string(job.Trust)) == "untrusted" {
		reasons = append(reasons, SecretsUntrusted)
	}
	if releaseSecret(job.SecretClasses) && RankTrust(DecodeTrust(string(job.Trust))) < RankTrust("internal") {
		reasons = append(reasons, TrustReleaseNeedsDedicated)
	}
	if runner.Hosted {
		reasons = append(reasons, SecretsNotInPolicy)
	}
	if releaseSecret(job.SecretClasses) && !runner.Dedicated && RankIsolation(runner.Isolation) < RankIsolation("vm") {
		reasons = append(reasons, TrustReleaseNeedsDedicated)
	}
	return reasons
}
