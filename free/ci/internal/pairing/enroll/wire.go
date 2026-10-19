// Package enroll is the enrollment listener and the join client (P7-NODE-07, Epic D5): one TLS 1.3
// route, POST /v1/enroll, open only for the pairing window and the only route that accepts a client
// without a certificate.
package enroll

// Purpose: the wire types shared by the listener and the joining node.
// Inputs:  JSON bodies, at most 16 KiB, decoded strictly (unknown fields are an error).
// Outputs: Request (phase hello or enroll), Hello, Response, ErrorBody.
// Constraints: phase "hello" carries no code and returns public facts only (the coordinator id and its
//          clock, so a node can refuse a skewed clock before it sends anything secret); phase "enroll"
//          carries the code, the CSR and the node key; the code field is never logged.

// Route is the one route of the listener.
const Route = "/v1/enroll"

// Version is the wire version.
const Version = 1

// MaxBody is the request body cap in bytes.
const MaxBody = 16 << 10

// Phases of Request.
const (
	PhaseHello  = "hello"
	PhaseEnroll = "enroll"
)

// Request is the body of POST /v1/enroll.
type Request struct {
	Phase      string `json:"phase"`
	Version    int    `json:"version"`
	ClientTime int64  `json:"client_time"` // unix seconds on the node
	Code       string `json:"code,omitempty"`
	// CSR is the DER certificate request; NodeKey the raw 32-byte Ed25519 public key.
	CSR     []byte `json:"csr,omitempty"`
	NodeKey []byte `json:"node_key,omitempty"`
	// NodeKeySig is the node key's signature over Transcript (proof that the joiner holds the private key).
	NodeKeySig   []byte `json:"node_key_sig,omitempty"`
	Name         string `json:"name,omitempty"`
	OS           string `json:"os,omitempty"`
	Arch         string `json:"arch,omitempty"`
	MachineID    string `json:"machine_id,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
}

// Hello is the reply to phase hello.
type Hello struct {
	Version       int    `json:"version"`
	CoordinatorID string `json:"coordinator_id"`
	ServerTime    int64  `json:"server_time"`
}

// Response is the reply to a successful phase enroll.
type Response struct {
	NodeID     string   `json:"node_id"`
	Cert       []byte   `json:"cert"`  // leaf DER
	Chain      [][]byte `json:"chain"` // leaf, node intermediate (DER)
	Root       []byte   `json:"root"`  // root DER, to be matched against the confirmed fingerprint
	AgentURL   string   `json:"agent_url"`
	NotAfter   int64    `json:"not_after"` // unix seconds
	ServerTime int64    `json:"server_time"`
}

// ErrorBody is the JSON body of every non-200 reply.
type ErrorBody struct {
	Code    string `json:"code"` // E659 or E660
	Reason  string `json:"reason"`
	Message string `json:"message"`
}
