package ssh

import "github.com/nself-org/plugins/free/ci/internal/model"

// Codes of the SSH push transport (Epic D15, P7-NODE-03).
const (
	CodeHostKey   = "E655"
	CodeTransport = "E656"
)

// Stable reason keys carried by Error.Reason.
const (
	ReasonHostKeyUnknown = "transport.ssh_host_key_unknown"
	ReasonHostKeyChanged = "transport.ssh_host_key_changed"
	ReasonSessionLost    = "transport.ssh_session_lost"
	ReasonConnect        = "transport.ssh_connect_failed"
	ReasonConnectTimeout = "transport.ssh_connect_timeout"
	ReasonNoDestination  = "transport.ssh_no_destination"
	ReasonPreamble       = "transport.ssh_preamble"
	ReasonHandshake      = "transport.ssh_handshake"
	ReasonCheckout       = "transport.ssh_checkout"
	ReasonBlob           = "transport.ssh_blob"
	ReasonFenced         = "transport.ssh_lease_fenced"
	ReasonRefused        = "transport.ssh_lease_refused"
)

func init() {
	model.Register(model.Code{ID: CodeHostKey, Class: "usage", Summary: "node host key unknown or changed",
		Fix: "Verify the node, then run `nself ci nodes rotate <node> --host-key SHA256:<fingerprint>` (or `nself ci nodes add` for a new node)"})
	model.Register(model.Code{ID: CodeTransport, Class: "infra", Summary: "ssh transport failure",
		Fix: "Check that the node is reachable and the agent is installed (`nself ci nodes test <node>`); the attempt is retried if it is retryable"})
}
