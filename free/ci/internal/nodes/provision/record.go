package provision

// Purpose: the registry record of a provisioned GitHub runner (P7-NODE-06, review F14). A runner takes
// its jobs from GitHub, not from an nSelf lease, so the node is recorded with trust.accepts null from
// source "provider": no trust class is ever admitted and the scheduler never offers it a lease.
// Inputs:  the host label of the provisioned machine and the runner labels.
// Outputs: a validated model.Capability document and the node id derived from the host.
// Constraints: the record holds the host name and the labels only, never the registration token, the
// GitHub URL or any host output; the id is lower case, `gh-runner-` plus the host with unsafe bytes
// replaced; the document passes model.ValidateCapability.

import (
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RecordRequest is what the composition root records after a provision succeeded.
type RecordRequest struct {
	// Host is the ssh target as given ("user@host").
	Host   string
	Labels []string
}

// RunnerNodeID is the registry id of the runner host.
func RunnerNodeID(host string) string {
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		host = host[i+1:]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(host) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return "gh-runner-" + strings.Trim(b.String(), "-.")
}

// RunnerCapability builds the capability document of a provisioned runner host.
func RunnerCapability(req RecordRequest, now time.Time) model.Capability {
	id := RunnerNodeID(req.Host)
	user, host := "", req.Host
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		user, host = host[:i], host[i+1:]
	}
	c := model.NewCapability("provider", "provider", now)
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.Provider, c.Identity.Transport = "ssh", "provider"
	c.Identity.Labels = append([]string{"github-runner"}, req.Labels...)
	c.Identity.Ownership = model.KnownFact("operator", "assigned", now)
	c.Identity.SSH = &model.CapabilitySSH{Alias: host, User: user}
	c.Platform.OS = model.KnownFact("linux", "assigned", now)
	c.Availability.State = model.KnownFact("online", "assigned", now)
	c.Availability.Persistence = model.KnownFact(true, "assigned", now)
	c.Separation = model.CapabilitySeparation{UIDSeparation: model.KnownFact(false, "assigned", now), UserNS: model.KnownFact(false, "assigned", now),
		SigningEligible: model.KnownFact(false, "assigned", now)}
	c.Verified = model.CapabilityVerified{By: "provider", At: now}
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1}
	c.Lifecycle.Discovered = true
	// null from source provider: the one shape ValidateCapability allows for an unknown accepts set.
	c.Trust.Accepts = model.UnknownFact[[]model.TrustClass]("provider", now)
	return c
}
