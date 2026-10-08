package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Capability is the stable ci.runner-capability/v1 document.
type Capability struct {
	Schema          string                 `json:"schema"`
	Identity        CapabilityIdentity     `json:"identity"`
	Platform        CapabilityPlatform     `json:"platform"`
	Resources       CapabilityResources    `json:"resources"`
	Tools           CapabilityTools        `json:"tools"`
	Availability    CapabilityAvailability `json:"availability"`
	Location        CapabilityLocation     `json:"location"`
	Economics       CapabilityEconomics    `json:"economics"`
	Separation      CapabilitySeparation   `json:"separation"`
	Trust           CapabilityTrust        `json:"trust"`
	Verified        CapabilityVerified     `json:"verified"`
	AgeRecipient    Fact[string]           `json:"age_recipient"`
	SecretsEligible bool                   `json:"secrets_eligible"`
	Protocol        CapabilityProtocol     `json:"protocol"`
	DeployHost      CapabilityDeployHost   `json:"deploy_host"`
	Lifecycle       CapabilityLifecycle    `json:"lifecycle"`
}

type CapabilityIdentity struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Provider            string         `json:"provider"`
	Transport           string         `json:"transport"`
	Ownership           Fact[string]   `json:"ownership"`
	Labels              []string       `json:"labels"`
	MachineID           Fact[string]   `json:"machine_id"`
	HostKeyFingerprints []string       `json:"host_key_fingerprints"`
	SSH                 *CapabilitySSH `json:"ssh"`
}
type CapabilitySSH struct {
	Alias    string `json:"alias"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	User     string `json:"user"`
}
type CapabilityPlatform struct {
	OS             Fact[string] `json:"os"`
	Arch           Fact[string] `json:"arch"`
	Kernel         Fact[string] `json:"kernel"`
	OSVersion      Fact[string] `json:"os_version"`
	Virtualization Fact[string] `json:"virtualization"`
}
type CapabilityResources struct {
	CPU          Fact[float64]      `json:"cpu"`
	MemMB        Fact[int64]        `json:"mem_mb"`
	DiskFreeMB   Fact[int64]        `json:"disk_free_mb"`
	Accelerators Fact[[]string]     `json:"accelerators"`
	Reserved     CapabilityReserved `json:"reserved"`
}
type CapabilityReserved struct {
	CPU   Fact[float64] `json:"cpu"`
	MemMB Fact[int64]   `json:"mem_mb"`
}
type CapabilityTools struct {
	Docker     Fact[bool]              `json:"docker"`
	Podman     Fact[bool]              `json:"podman"`
	GVisor     Fact[bool]              `json:"gvisor"`
	Tart       Fact[bool]              `json:"tart"`
	Toolchains map[string]Fact[string] `json:"toolchains"`
	Browsers   Fact[[]string]          `json:"browsers"`
	Services   Fact[[]string]          `json:"services"`
	Custom     map[string]Fact[string] `json:"custom"`
}
type CapabilityAvailability struct {
	State             Fact[string] `json:"state"`
	Slots             Fact[int]    `json:"slots"`
	Running           Fact[int]    `json:"running"`
	BatteryPct        Fact[int]    `json:"battery_pct"`
	PluggedIn         Fact[bool]   `json:"plugged_in"`
	Interactive       Fact[string] `json:"interactive"`
	Persistence       Fact[bool]   `json:"persistence"`
	OnBattery         Fact[string] `json:"on_battery"`
	InteractivePolicy Fact[string] `json:"interactive_policy"`
}
type CapabilityLocation struct {
	Kind   Fact[string] `json:"kind"`
	Region Fact[string] `json:"region"`
}
type CapabilityEconomics struct {
	Class              Fact[string]        `json:"class"`
	MarginalCostPerMin Fact[float64]       `json:"marginal_cost_per_min"`
	Allowance          CapabilityAllowance `json:"allowance"`
}
type CapabilityAllowance struct {
	KnownRemaining Fact[float64]   `json:"known_remaining"`
	ResetAt        Fact[time.Time] `json:"reset_at"`
	Confidence     Fact[string]    `json:"confidence"`
}
type CapabilitySeparation struct {
	UIDSeparation   Fact[bool] `json:"uid_separation"`
	UserNS          Fact[bool] `json:"userns"`
	SigningEligible Fact[bool] `json:"signing_eligible"`
}
type CapabilityTrust struct {
	Accepts       Fact[[]TrustClass]   `json:"accepts"`
	Isolation     Fact[Isolation]      `json:"isolation"`
	Network       Fact[[]NetworkScope] `json:"network"`
	SecretClasses Fact[[]SecretClass]  `json:"secret_classes"`
	PrivacyZone   Fact[PrivacyZone]    `json:"privacy_zone"`
}
type CapabilityVerified struct {
	By string    `json:"by"`
	At time.Time `json:"at"`
}
type CapabilityProtocol struct {
	Versions     []int  `json:"versions"`
	Min          int    `json:"min"`
	AgentVersion string `json:"agent_version"`
}
type CapabilityDeployHost struct {
	Matched  bool    `json:"matched"`
	Source   *string `json:"source"`
	Override bool    `json:"override"`
}
type CapabilityLifecycle struct {
	Discovered         bool       `json:"discovered"`
	AuthorizedProjects []string   `json:"authorized_projects"`
	Eligible           bool       `json:"eligible"`
	RevokedAt          *time.Time `json:"revoked_at"`
}

// ValidateCapability checks the public schema, then cross-field invariants.
func ValidateCapability(c Capability) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	s, err := CapabilitySchema()
	if err != nil {
		return err
	}
	if err := ValidateJSON(s, b); err != nil {
		return err
	}
	if c.Trust.Accepts.Value == nil && c.Trust.Accepts.Source != "provider" {
		return fmt.Errorf("/trust/accepts: null requires provider source")
	}
	if c.Separation.UIDSeparation.Value == nil || !*c.Separation.UIDSeparation.Value {
		if c.SecretsEligible {
			return fmt.Errorf("/secrets_eligible: uid separation required")
		}
	}
	return nil
}
