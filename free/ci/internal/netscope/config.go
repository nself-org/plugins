package netscope

// Purpose: the configuration the planner hands to the sidecar and the sidecar reads at start.
// Inputs:  the scope, the extra allowlist hosts, the deny Facts the host computed, the job network prefix.
// Outputs: Config, encoded to one JSON string that travels in the sidecar's environment (never on argv).
// Constraints: the config carries no secret; it is bounded (maxConfigBytes) so an oversized world fails the
// attempt with E680 instead of truncating a deny list.

import (
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ConfigEnv is the sidecar environment variable that holds the encoded Config.
const ConfigEnv = "NSELF_NETPROXY_CONFIG"

// ProxyPort is the port the sidecar listens on, on its job-network address only.
const ProxyPort = 3128

// maxConfigBytes bounds the encoded config (an environment value; Linux caps one string at 128 KiB).
const maxConfigBytes = 96 * 1024

// Config is the sidecar's whole input.
type Config struct {
	Scope   model.NetworkScope `json:"scope"`
	Attempt string             `json:"attempt"`
	// JobPrefix is the job network's subnet: the sidecar listens only on its address inside it.
	JobPrefix string   `json:"job_prefix"`
	Hosts     []string `json:"hosts,omitempty"`
	OnLink    []string `json:"on_link,omitempty"`
	HostAddrs []string `json:"host_addrs,omitempty"`
	Exclusion []string `json:"exclusion,omitempty"`
	// ExclusionHosts are deploy exclusion host names.
	ExclusionHosts []string `json:"exclusion_hosts,omitempty"`
}

// NewConfig builds a Config from Facts.
func NewConfig(scope model.NetworkScope, attempt string, jobPrefix netip.Prefix, extraHosts []string, f Facts) Config {
	c := Config{Scope: scope, Attempt: attempt, JobPrefix: jobPrefix.String(), Hosts: extraHosts, ExclusionHosts: f.ExclusionHosts}
	for _, p := range f.OnLink {
		c.OnLink = append(c.OnLink, p.String())
	}
	for _, a := range f.HostAddrs {
		c.HostAddrs = append(c.HostAddrs, a.String())
	}
	for _, p := range f.Exclusion {
		c.Exclusion = append(c.Exclusion, p.String())
	}
	return c
}

// Encode renders the config for the environment.
func (c Config) Encode() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	if len(b) > maxConfigBytes {
		return "", scopeFailed("the proxy configuration is %d bytes, above the %d limit", len(b), maxConfigBytes)
	}
	return string(b), nil
}

// DecodeConfig parses the encoded config and returns it with the Facts it holds. Any entry that does not
// parse fails the whole config: a deny list is never half applied.
func DecodeConfig(s string) (Config, Facts, error) {
	var c Config
	if len(s) > maxConfigBytes {
		return c, Facts{}, scopeFailed("the proxy configuration is above the %d byte limit", maxConfigBytes)
	}
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return c, Facts{}, scopeFailed("the proxy configuration is not valid: %v", err)
	}
	if c.Scope != "restricted" && c.Scope != "internet" {
		return c, Facts{}, scopeFailed("the proxy serves restricted and internet only, not %q", c.Scope)
	}
	var f Facts
	for _, raw := range c.OnLink {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return c, f, scopeFailed("on-link prefix %q: %v", raw, err)
		}
		f.OnLink = append(f.OnLink, p)
	}
	for _, raw := range c.Exclusion {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return c, f, scopeFailed("exclusion prefix %q: %v", raw, err)
		}
		f.Exclusion = append(f.Exclusion, p)
	}
	for _, raw := range c.ExclusionHosts {
		if h := NormalizeHost(raw); h != "" {
			f.ExclusionHosts = append(f.ExclusionHosts, h)
		}
	}
	for _, raw := range c.HostAddrs {
		a, err := netip.ParseAddr(raw)
		if err != nil {
			return c, f, scopeFailed("host address %q: %v", raw, err)
		}
		f.HostAddrs = append(f.HostAddrs, a)
	}
	if _, err := netip.ParsePrefix(c.JobPrefix); err != nil {
		return c, f, scopeFailed("job prefix %q: %v", c.JobPrefix, err)
	}
	return c, f, nil
}

// String names the config without its lists, for logs.
func (c Config) String() string {
	return fmt.Sprintf("scope=%s attempt=%s job_prefix=%s hosts=%d", c.Scope, c.Attempt, c.JobPrefix, len(c.Hosts))
}
