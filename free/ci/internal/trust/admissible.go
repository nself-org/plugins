package trust

import "github.com/nself-org/plugins/free/ci/internal/model"

type Running struct {
	Untrusted     bool
	SecretBearing bool
}
type DeployHost struct {
	Matched  bool
	Override bool
}
type Declaration struct {
	Accepts         []model.TrustClass
	Isolation       model.Isolation
	Network         model.NetworkScope
	SecretClasses   []model.SecretClass
	PrivacyZone     model.PrivacyZone
	Ownership       string
	Dedicated       bool
	NoLANRoute      bool
	Hosted          bool
	Provider        string
	UIDSeparation   bool
	Running         Running
	DeployHost      DeployHost
	CoordinatorHost bool
	CoordinatorMode CoordinatorMode
	Source          string
	Projects        int
	Shared          bool
	PersonalOwnNode bool
	PersonalOwnRun  bool
	ReleaseJob      bool
}
type admissionRule func(JobFacts, Declaration, Effective) []Reason

var AdmissionRules = []admissionRule{admitAuthorization, admitPrivacy, admitTrust, admitIsolation, admitNetwork, admitSecrets}

func containsTrust(values []model.TrustClass, want model.TrustClass) bool {
	for _, x := range values {
		if x == want {
			return true
		}
	}
	return false
}
func containsSecret(values []model.SecretClass, want model.SecretClass) bool {
	for _, x := range values {
		if x == want {
			return true
		}
	}
	return false
}
func containsString(values []string, want string) bool {
	for _, x := range values {
		if x == want {
			return true
		}
	}
	return false
}
func policyString(e Effective, key string) string {
	v, _, ok := e.Value(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
func policySet(e Effective, key string) []string {
	v, _, ok := e.Value(key)
	if !ok {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := []string{}
		for _, y := range x {
			if s, ok := y.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func Admissible(job JobFacts, declaration Declaration, policy Effective) []Reason {
	var out []Reason
	for _, rule := range AdmissionRules {
		out = append(out, rule(job, declaration, policy)...)
	}
	return out
}
func admitAuthorization(job JobFacts, d Declaration, _ Effective) []Reason {
	if !containsTrust(d.Accepts, DecodeTrust(string(job.Trust))) {
		return []Reason{TrustNotAccepted}
	}
	return nil
}
func admitPrivacy(job JobFacts, d Declaration, e Effective) []Reason {
	zone := DecodePrivacy(policyString(e, "trust.privacy_zone"))
	if _, _, ok := e.Value("trust.privacy_zone"); !ok {
		zone = "private-infrastructure"
	}
	if zone == "local-only" && !d.PersonalOwnNode {
		return []Reason{PrivacyLocalOnly}
	}
	if !d.Hosted {
		return nil
	}
	authorized := e.hostedProject || e.hostedPublic && DecodeTrust(string(job.Trust)) == "untrusted"
	if !authorized {
		return []Reason{PrivacyHostedNotAuthorized}
	}
	switch zone {
	case "local-only":
		return []Reason{PrivacyLocalOnly}
	case "private-infrastructure":
		return []Reason{PrivacyHostedNotAuthorized}
	case "provider-allowlist":
		if !containsString(policySet(e, "trust.providers"), d.Provider) {
			return []Reason{PrivacyProviderNotAllowlisted}
		}
	}
	return nil
}
func admitTrust(job JobFacts, d Declaration, _ Effective) []Reason {
	if releaseSecret(job.SecretClasses) && RankTrust(DecodeTrust(string(job.Trust))) < RankTrust("internal") {
		return []Reason{TrustReleaseNeedsDedicated}
	}
	if releaseSecret(job.SecretClasses) && RankIsolation(d.Isolation) < RankIsolation("vm") && !IsDedicatedTrusted(d) {
		return []Reason{TrustReleaseNeedsDedicated}
	}
	return nil
}
func admitIsolation(job JobFacts, d Declaration, e Effective) []Reason {
	var out []Reason
	min := MinIsolation(job, RunnerFacts{Ownership: d.Ownership, Dedicated: IsDedicatedTrusted(d), Hosted: d.Hosted, UIDSeparation: d.UIDSeparation, PersonalOwnNode: d.PersonalOwnNode, NoLANRoute: d.NoLANRoute})
	if v := policyString(e, "trust.isolation_min"); v != "" && RankIsolation(DecodeIsolation(v)) > RankIsolation(min) {
		min = DecodeIsolation(v)
	}
	if RankIsolation(d.Isolation) < RankIsolation(min) {
		out = append(out, IsolationBelowMinimum)
	}
	ownerException := DecodeTrust(string(job.Trust)) == "owner" && d.Ownership == "operator" && d.PersonalOwnNode && d.Isolation == "process" && DecodeCoordinatorMode(string(d.CoordinatorMode)) == Personal && job.PersonalOwnRun
	if !d.UIDSeparation && !ownerException {
		out = append(out, IsolationUIDSeparation)
	}
	if DecodeTrust(string(job.Trust)) == "untrusted" && d.Running.SecretBearing || hasSecret(job.SecretClasses) && d.Running.Untrusted {
		out = append(out, IsolationCotenancy)
	}
	return out
}
func admitNetwork(job JobFacts, d Declaration, e Effective) []Reason {
	var out []Reason
	if RankNetwork(job.Network) < 0 || RankNetwork(job.Network) > RankNetwork(d.Network) {
		out = append(out, NetworkScopeNotOffered)
	}
	if v := policyString(e, "trust.network"); v != "" && RankNetwork(job.Network) > RankNetwork(DecodeNetwork(v)) {
		out = append(out, NetworkScopeNotOffered)
	}
	if DecodeTrust(string(job.Trust)) == "untrusted" && ((!d.NoLANRoute && d.Isolation != "hosted-disposable") || RankNetwork(job.Network) >= RankNetwork("lan")) {
		out = append(out, NetworkLANForUntrusted)
	}
	return out
}
func admitSecrets(job JobFacts, d Declaration, e Effective) []Reason {
	var out []Reason
	if d.Hosted && hasSecret(job.SecretClasses) {
		out = append(out, SecretsNotInPolicy)
	}
	if DecodeTrust(string(job.Trust)) == "untrusted" && hasSecret(job.SecretClasses) {
		out = append(out, SecretsUntrusted)
	}
	_, _, hasPolicy := e.Value("trust.secrets")
	allowed := policySet(e, "trust.secrets")
	for _, class := range job.SecretClasses {
		if class == "none" {
			continue
		}
		if !containsSecret(d.SecretClasses, class) {
			out = append(out, SecretsClassNotDeclared)
		}
		if hasPolicy && !containsString(allowed, string(class)) {
			out = append(out, SecretsNotInPolicy)
		}
	}
	return out
}
