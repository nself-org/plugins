package secrets

// Purpose: gate the one value-bearing carrier form, the NODE-02 `secrets` message (X8).
// Inputs:  the coordinator mode, the carrier, the node's ownership and the secret class.
// Outputs: true only when a value may travel to the node; otherwise the coordinator sends refs.
// Constraints: pure and fail closed: an unknown mode, carrier, ownership or class is false. The
// only true combination is (personal, ssh, operator, project|environment).

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// CarrierSSH is carrier A, the SSH stdio carrier (`transport: ssh`).
const CarrierSSH = "ssh"

// CarrierValueAllowed reports whether a coordinator may send a secret value (not a ref) over carrier.
func CarrierValueAllowed(mode trust.CoordinatorMode, carrier, ownership string, class model.SecretClass) bool {
	if mode != trust.Personal || carrier != CarrierSSH || ownership != "operator" {
		return false
	}
	return class == "project" || class == "environment"
}
