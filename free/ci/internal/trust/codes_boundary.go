package trust

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	model.Register(model.Code{ID: "E689", Class: "usage", Summary: "placement refused at the production boundary", Fix: "Run the job on a node that is not a deploy host, or on a deploy host the operator overrode with owner or internal work, isolation vm or stronger and no release or deploy secrets; a coordinator host needs vm for team mode and for collaborator or untrusted work"})
}
