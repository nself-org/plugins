package trust

type Reason string

const (
	TrustNotAccepted              Reason = "trust.not_accepted"
	TrustReleaseNeedsDedicated    Reason = "trust.release_needs_dedicated"
	IsolationBelowMinimum         Reason = "isolation.below_minimum"
	IsolationUnavailable          Reason = "isolation.unavailable"
	IsolationUIDSeparation        Reason = "isolation.uid_separation"
	IsolationCotenancy            Reason = "isolation.cotenancy"
	IsolationSlotUIDShared        Reason = "isolation.slot_uid_shared"
	IsolationRootfulRuntime       Reason = "isolation.rootful_runtime"
	IsolationRuntimeFlagDenied    Reason = "isolation.runtime_flag_denied"
	NetworkScopeNotOffered        Reason = "network.scope_not_offered"
	NetworkLANForUntrusted        Reason = "network.lan_for_untrusted"
	NetworkScopeUnenforceable     Reason = "network.scope_unenforceable"
	SecretsUntrusted              Reason = "secrets.untrusted"
	SecretsClassNotDeclared       Reason = "secrets.class_not_declared"
	SecretsNotInPolicy            Reason = "secrets.not_in_policy"
	PrivacyHostedNotAuthorized    Reason = "privacy.hosted_not_authorized"
	PrivacyProviderNotAllowlisted Reason = "privacy.provider_not_allowlisted"
	PrivacyLocalOnly              Reason = "privacy.local_only"
	BoundaryDeployHost            Reason = "boundary.deploy_host"
	BoundaryOverrideMinimum       Reason = "boundary.override_minimum"
	BoundaryCoordinatorHost       Reason = "boundary.coordinator_host"
	ApprovalRequired              Reason = "approval.required"
)

var ReasonMeaning = map[Reason]string{
	TrustNotAccepted: "runner does not accept this trust class", TrustReleaseNeedsDedicated: "release secrets need a dedicated runner or VM", IsolationBelowMinimum: "isolation below minimum", IsolationUnavailable: "isolation unavailable", IsolationUIDSeparation: "node lacks UID separation", IsolationCotenancy: "untrusted and secret-bearing work overlap", IsolationSlotUIDShared: "job slot shares a UID", IsolationRootfulRuntime: "rootful runtime is unavailable", IsolationRuntimeFlagDenied: "runtime flag denied", NetworkScopeNotOffered: "network scope not offered", NetworkLANForUntrusted: "untrusted work has a LAN route", NetworkScopeUnenforceable: "network scope cannot be enforced", SecretsUntrusted: "untrusted work requests secrets", SecretsClassNotDeclared: "secret class not declared", SecretsNotInPolicy: "secrets disallowed by policy", PrivacyHostedNotAuthorized: "hosted execution not authorized", PrivacyProviderNotAllowlisted: "provider not allowlisted", PrivacyLocalOnly: "local-only work must stay local", BoundaryDeployHost: "deploy host ineligible", BoundaryOverrideMinimum: "deploy override minimum violated", BoundaryCoordinatorHost: "coordinator host floor violated", ApprovalRequired: "approval required",
}
