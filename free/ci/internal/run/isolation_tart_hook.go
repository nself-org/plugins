package run

// Purpose: register the Tart VM profile (`vm` and `ephemeral-vm`) with the isolation registry (P7-TRUST-06, Epic D10).
// Inputs:  the operator's ~/.nself/ci/config.yaml (or $NSELF_CI_HOME/config.yaml): ci.tart.base_image (digest
//          required), ci.tart.ssh_user, ci.tart.ssh_key; the netscope planner's exclusion and host sources.
// Outputs: two registered isolation.Profile values. Registering does not make a level available: the profile
//          probes Tart, the platform, the base image digest and the free VM slots on every selection.
// Constraints: one file per profile (Epic D7); the base image comes only from operator scope, no default is
//          shipped; the config is re-read on each probe so a changed image applies without a restart.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/tart"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"gopkg.in/yaml.v3"
)

// tartSetting reads ci.tart.<key> from the operator config: nested (ci: tart: key:) or dotted (ci.tart.key:).
func tartSetting(key string) func() string {
	return func() string {
		home := os.Getenv("NSELF_CI_HOME")
		if home == "" {
			u, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			home = filepath.Join(u, ".nself", "ci")
		}
		data, err := os.ReadFile(filepath.Join(home, "config.yaml"))
		if err != nil {
			return ""
		}
		var doc map[string]any
		if yaml.Unmarshal(data, &doc) != nil {
			return ""
		}
		if v, ok := doc["ci.tart."+key].(string); ok {
			return strings.TrimSpace(v)
		}
		ci, _ := doc["ci"].(map[string]any)
		t, _ := ci["tart"].(map[string]any)
		v, _ := t[key].(string)
		return strings.TrimSpace(v)
	}
}

func init() {
	opts := tart.Options{
		BaseImage: tartSetting("base_image"),
		SSHUser:   tartSetting("ssh_user"),
		SSHKey:    tartSetting("ssh_key"),
		Net: tart.NetSource{
			Facts: tart.BuildFacts(netscope.Discover, netscopePlanner.Exclusion, nil),
			Hosts: func(a isolation.Attempt) []string {
				if netscopePlanner.ExtraHosts == nil {
					return nil
				}
				return netscopePlanner.ExtraHosts(a)
			},
			Proxy: tart.HostProxy(nil, nil, nil),
		},
	}
	isolation.Register(tart.New("vm", opts))
	isolation.Register(tart.New("ephemeral-vm", opts))
}
