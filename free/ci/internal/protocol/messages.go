package protocol

import (
	"encoding/json"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

type CompatMode string

const (
	Compat14 CompatMode = "v1.4"
	Compat15 CompatMode = "v1.5"
)

type CarrierPolicy struct {
	Carrier       string
	Personal      bool
	OperatorOwned bool
}
type NodeKey struct {
	Alg    string `json:"alg"`
	KeyID  string `json:"key_id"`
	Public string `json:"public"`
}
type Identity struct {
	MachineID           string   `json:"machine_id"`
	HostKeyFingerprints []string `json:"host_key_fingerprints"`
}
type Slots struct {
	Total int `json:"total"`
	Used  int `json:"used"`
}
type Hello struct {
	Versions            []int        `json:"versions"`
	AgentVersion        string       `json:"agent_version"`
	AgeRecipient        *string      `json:"age_recipient"`
	NodeKey             NodeKey      `json:"node_key"`
	CapabilityDigest    string       `json:"capability_digest"`
	Identity            Identity     `json:"identity"`
	CompatModeSupported []CompatMode `json:"compat_mode_supported"`
	Slots               Slots        `json:"slots"`
}
type Welcome struct {
	Version         int    `json:"version"`
	MinAgentVersion string `json:"min_agent_version"`
	SessionID       string `json:"session_id"`
	HeartbeatMS     int    `json:"heartbeat_ms"`
	AgentTTLMS      int    `json:"agent_ttl_ms"`
}
type Reject struct {
	Code            string `json:"code"`
	Reason          string `json:"reason"`
	MinAgentVersion string `json:"min_agent_version"`
}
type LeaseState struct {
	LeaseID string `json:"lease_id"`
	Epoch   uint64 `json:"epoch"`
	State   string `json:"state"`
}
type Heartbeat struct {
	Leases      []LeaseState `json:"leases"`
	Slots       Slots        `json:"slots"`
	Battery     *bool        `json:"battery"`
	Interactive bool         `json:"interactive"`
}
type Checkout struct {
	Revision     string `json:"revision"`
	BundleDigest string `json:"bundle_digest"`
	Size         int64  `json:"size"`
}
type SecretRef struct {
	Ref   string            `json:"ref"`
	Class model.SecretClass `json:"class"`
}
type SecretValue struct {
	Ref   string `json:"ref"`
	Value string `json:"value"`
}
type Secrets struct {
	LeaseID string        `json:"lease_id"`
	Values  []SecretValue `json:"values"`
}
type RunnerCredential struct {
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	ExpiresAt string `json:"expires_at"`
}
type CacheBlob struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type CacheRestore struct {
	Spec  string      `json:"spec"`
	Key   string      `json:"key"`
	Dir   string      `json:"dir"`
	Blobs []CacheBlob `json:"blobs"`
}
type CacheSave struct {
	Spec string   `json:"spec"`
	Key  string   `json:"key"`
	Dirs []string `json:"dirs"`
}
type LeaseCache struct {
	Namespace string         `json:"namespace"`
	Access    string         `json:"access"`
	Restore   []CacheRestore `json:"restore"`
	Save      []CacheSave    `json:"save"`
}
type Lease struct {
	LeaseID          string            `json:"lease_id"`
	AttemptID        string            `json:"attempt_id"`
	Epoch            uint64            `json:"epoch"`
	Token            string            `json:"token"`
	CompatMode       CompatMode        `json:"compat_mode"`
	Job              model.Job         `json:"job"`
	Checkout         Checkout          `json:"checkout"`
	Secrets          []SecretRef       `json:"secrets"`
	ExpiresAt        string            `json:"expires_at"`
	Cache            *LeaseCache       `json:"cache,omitempty"`
	RunnerCredential *RunnerCredential `json:"runner_credential,omitempty"`
}
type Chunk struct {
	Stream string `json:"stream"`
	Offset int64  `json:"offset"`
	Data   string `json:"data"`
	Digest string `json:"digest,omitempty"`
	Final  bool   `json:"final"`
}
type Log struct {
	LeaseID string `json:"lease_id"`
	LineSeq uint64 `json:"line_seq"`
	Stream  string `json:"stream"`
	Text    string `json:"text"`
}
type ArtifactRef struct {
	LeaseID string `json:"lease_id"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}
type Signature struct {
	KeyID string `json:"key_id"`
	Value string `json:"value"`
}
type Result struct {
	LeaseID   string          `json:"lease_id"`
	Epoch     uint64          `json:"epoch"`
	Evidence  json.RawMessage `json:"evidence"`
	Signature Signature       `json:"signature"`
}
type Cancel struct {
	LeaseID string `json:"lease_id"`
	Reason  string `json:"reason"`
}
type Drain struct {
	Deadline string `json:"deadline"`
}
type Upgrade struct {
	Version     string `json:"version"`
	ManifestURL string `json:"manifest_url"`
}
type Ack struct {
	AckSeq  uint64 `json:"ack_seq"`
	LeaseID string `json:"lease_id,omitempty"`
}
type Digest struct {
	Stream string `json:"stream"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Offset struct {
	Stream string `json:"stream"`
	SHA256 string `json:"sha256"`
	Offset int64  `json:"offset"`
}
type Resume struct {
	LastSeq uint64 `json:"last_seq"`
}
type CacheRef struct {
	LeaseID string      `json:"lease_id"`
	Spec    string      `json:"spec"`
	Key     string      `json:"key"`
	Blobs   []CacheBlob `json:"blobs"`
}
