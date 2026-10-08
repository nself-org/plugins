package trust

import "github.com/nself-org/plugins/free/ci/internal/model"

var trustOrder = []string{"untrusted", "collaborator", "internal", "owner"}
var isolationOrder = []string{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"}
var networkOrder = []string{"none", "restricted", "internet", "lan", "privileged"}
var privacyOrder = []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}

func rank(order []string, value string) int {
	for i, item := range order {
		if item == value {
			return i
		}
	}
	return -1
}

func RankTrust(v model.TrustClass) int { return rank(trustOrder, string(v)) }
func RankIsolation(v model.Isolation) int {
	if v == "hosted-disposable" {
		return 4
	}
	return rank(isolationOrder, string(v))
}
func RankNetwork(v model.NetworkScope) int { return rank(networkOrder, string(v)) }
func RankPrivacy(v model.PrivacyZone) int  { return rank(privacyOrder, string(v)) }

func DecodeTrust(v string) model.TrustClass {
	if rank(trustOrder, v) < 0 {
		return "untrusted"
	}
	return model.TrustClass(v)
}
func DecodeIsolation(v string) model.Isolation {
	if v == "hosted-disposable" {
		return model.Isolation(v)
	}
	if rank(isolationOrder, v) < 0 {
		return "process"
	}
	return model.Isolation(v)
}
func DecodeNetwork(v string) model.NetworkScope {
	if rank(networkOrder, v) < 0 {
		return "none"
	}
	return model.NetworkScope(v)
}
func DecodePrivacy(v string) model.PrivacyZone {
	if rank(privacyOrder, v) < 0 {
		return "local-only"
	}
	return model.PrivacyZone(v)
}

type CoordinatorMode string

const (
	Personal CoordinatorMode = "personal"
	Team     CoordinatorMode = "team"
)

func DecodeCoordinatorMode(v string) CoordinatorMode {
	if v == string(Personal) {
		return Personal
	}
	return Team
}
