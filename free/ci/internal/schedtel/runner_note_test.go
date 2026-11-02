package schedtel

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

func strFact(v string) model.Fact[string] { return model.Fact[string]{Value: &v, Source: "probe"} }

func nodeRunner(id, transport, location, os, arch string, iso model.Isolation) sched.Runner {
	c := model.Capability{}
	c.Identity = model.CapabilityIdentity{ID: id, Provider: "ssh", Transport: transport}
	if os != "" {
		c.Platform.OS, c.Platform.Arch = strFact(os), strFact(arch)
	}
	if iso != "" {
		c.Trust.Isolation = model.Fact[model.Isolation]{Value: &iso, Source: "probe"}
	}
	return sched.Runner{ID: id, Capability: c, Provider: "ssh", Location: location}
}

// TestAttemptRunner: the runner an admitted attempt records is the one the dispatcher chose, with
// the platform and isolation it declares; what it does not declare is stated as the weakest claim.
func TestAttemptRunner(t *testing.T) {
	w := &sched.World{Runners: []sched.Runner{
		nodeRunner("here", "local", "local", "darwin", "arm64", "process"),
		nodeRunner("lab-arm", "ssh", "lan", "linux", "arm64", "container"),
		nodeRunner("pair", "agent", "vps", "linux", "amd64", "vm"),
		nodeRunner("bare", "ssh", "lan", "", "", ""),
	}, Hosted: []sched.Runner{nodeRunner("gh", "rest", "hosted", "linux", "amd64", "hosted-disposable")}}
	for id, want := range map[string]model.AttemptRunner{
		"here":    {Kind: "local", Tier: "local", Node: "here", Provider: "ssh", Platform: "darwin/arm64", Isolation: "process"},
		"lab-arm": {Kind: "ssh", Tier: "lan", Node: "lab-arm", Provider: "ssh", Platform: "linux/arm64", Isolation: "container"},
		"pair":    {Kind: "agent", Tier: "vps", Node: "pair", Provider: "ssh", Platform: "linux/amd64", Isolation: "vm"},
		"bare":    {Kind: "ssh", Tier: "lan", Node: "bare", Provider: "ssh", Platform: "unknown", Isolation: "process"},
		"gh":      {Kind: "hosted", Tier: "hosted", Node: "gh", Provider: "ssh", Platform: "linux/amd64", Isolation: "hosted-disposable"},
	} {
		got, ok := attemptRunner(w, id)
		if !ok || got != want {
			t.Errorf("%s: %+v (found %v), want %+v", id, got, ok, want)
		}
	}
	if _, ok := attemptRunner(w, "gone"); ok {
		t.Error("a runner the world does not hold must not be invented")
	}
	if _, ok := attemptRunner(nil, "here"); ok {
		t.Error("no world, no runner")
	}
}
