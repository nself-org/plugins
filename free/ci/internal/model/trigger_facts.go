package model

import (
	"fmt"
	"time"
)

// FactSource identifies the authority from which a trigger claim came.
type FactSource string

const (
	FactPayload FactSource = "payload"
	FactAPI     FactSource = "api"
	FactCaller  FactSource = "caller"
	FactLocal   FactSource = "local"
)

type TriggerKind string
type TriggerAuth string

// TriggerFacts is the normalized, source attributed input to trust classification.
// Fact reuses the runner capability value wrapper; absent claims have nil Value.
type TriggerFacts struct {
	Schema             string         `json:"schema"`
	Source             string         `json:"source"`
	Kind               TriggerKind    `json:"kind"`
	DeliveryID         string         `json:"delivery_id,omitempty"`
	ReceivedAt         time.Time      `json:"received_at"`
	Auth               TriggerAuth    `json:"auth"`
	Repo               string         `json:"repo"`
	RepoID             string         `json:"repo_id,omitempty"`
	Ref                string         `json:"ref,omitempty"`
	Revision           string         `json:"revision,omitempty"`
	RevisionSource     FactSource     `json:"revision_source,omitempty"`
	BaseRef            string         `json:"base_ref,omitempty"`
	BaseRevision       string         `json:"base_revision,omitempty"`
	PRNumber           int            `json:"pr_number,omitempty"`
	Actor              Fact[string]   `json:"actor"`
	ActorPermission    Fact[string]   `json:"actor_permission"`
	AuthorAssociation  Fact[string]   `json:"author_association"`
	HeadRepo           Fact[string]   `json:"head_repo"`
	Fork               Fact[bool]     `json:"fork"`
	FirstTime          Fact[bool]     `json:"first_time"`
	Bot                Fact[bool]     `json:"bot"`
	ProtectedRef       Fact[bool]     `json:"protected_ref"`
	Visibility         Fact[string]   `json:"visibility"`
	SHAReachable       Fact[bool]     `json:"sha_reachable"`
	HeadCurrent        Fact[bool]     `json:"head_current"`
	ChangedFiles       Fact[[]string] `json:"changed_files"`
	GroupProtectedOnly Fact[bool]     `json:"group_protected_only"`
	RunEvent           Fact[string]   `json:"run_event"`
	RunnerLabels       Fact[[]string] `json:"runner_labels"`
	ProviderJobID      Fact[string]   `json:"provider_job_id"`
	ProviderRunID      Fact[string]   `json:"provider_run_id"`
}

// Validate rejects unrecognized classifications and fact provenance.
func (f TriggerFacts) Validate() error {
	if f.Schema != "ci.trigger-facts/v1" {
		return fmt.Errorf("trigger facts: unknown schema")
	}
	if !oneOf(string(f.Kind), "push", "pull_request", "tag", "schedule", "manual", "api", "runner_demand") {
		return fmt.Errorf("trigger facts: unknown kind")
	}
	if !oneOf(string(f.Auth), "per-repo", "shared", "bearer", "none") {
		return fmt.Errorf("trigger facts: unknown auth")
	}
	if f.RevisionSource != "" && !oneOf(string(f.RevisionSource), "payload", "caller") {
		return fmt.Errorf("trigger facts: unknown revision source")
	}
	for _, err := range []error{validateFact(f.Actor), validateFact(f.ActorPermission), validateFact(f.AuthorAssociation), validateFact(f.HeadRepo), validateFact(f.Fork), validateFact(f.FirstTime), validateFact(f.Bot), validateFact(f.ProtectedRef), validateFact(f.Visibility), validateFact(f.SHAReachable), validateFact(f.HeadCurrent), validateFact(f.ChangedFiles), validateFact(f.GroupProtectedOnly), validateFact(f.RunEvent), validateFact(f.RunnerLabels), validateFact(f.ProviderJobID), validateFact(f.ProviderRunID)} {
		if err != nil {
			return err
		}
	}
	return nil
}

func validateFact[T any](f Fact[T]) error {
	if f.Value != nil && f.Source == "" {
		return fmt.Errorf("trigger facts: populated fact missing source")
	}
	if f.Source != "" && !oneOf(f.Source, "payload", "api", "caller", "local") {
		return fmt.Errorf("trigger facts: unknown fact source %q", f.Source)
	}
	return nil
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}
