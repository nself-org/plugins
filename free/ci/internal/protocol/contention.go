package protocol

// Purpose: the node contention policy on the wire (P7-NODE-13, D-1226): the coordinator hands a pull
//          node its stored reservation in welcome, and the agent names the node.* reason of a lease it
//          refused in its heartbeat.
// Inputs:  the agent's hello (CompatContention in compat_mode_supported).
// Outputs: Welcome.Contention; LeaseState.Reason.
// Constraints: both fields are optional and sent only to a peer that asked for them: the agent lists
//          CompatContention in hello, and only a coordinator that saw it answers with welcome.contention
//          (an older agent decodes welcome strictly and would reject the field); the agent names a reason
//          only after such a welcome (an older coordinator decodes heartbeats strictly). An older
//          coordinator ignores the extra compat entry, so nothing changes for it.

// CompatContention in hello's compat_mode_supported says the agent reads welcome.contention and may name
// the node.* reason of a refused lease state. It is a capability marker, never a lease compat_mode.
const CompatContention CompatMode = "contention.v1"

// Contention is the welcome's contention block. A non-nil block tells the agent the coordinator reads
// LeaseState.Reason; Policy is nil when the operator never set one for this node.
type Contention struct {
	Policy *NodePolicy `json:"policy,omitempty"`
}

// NodePolicy is the node's stored contention settings (store.NodeSettings without its timestamp).
type NodePolicy struct {
	Node          string  `json:"node"`
	ReservedCPU   float64 `json:"reserved_cpu"`
	ReservedMemMB int64   `json:"reserved_mem_mb"`
	Interactive   string  `json:"interactive"`
	OnBattery     string  `json:"on_battery"`
	Persistence   string  `json:"persistence"`
	Version       int64   `json:"version"`
}

// WantsContention reports whether hello asked for the contention block.
func WantsContention(h Hello) bool {
	for _, m := range h.CompatModeSupported {
		if m == CompatContention {
			return true
		}
	}
	return false
}
