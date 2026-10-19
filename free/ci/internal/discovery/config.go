package discovery

// Purpose: is LAN discovery switched off? Two switches, both from the operator: the environment
//          variable NSELF_CI_DISCOVERY=0 and `ci.discovery: off` in the user config.yaml.
// Inputs:  the environment and $NSELF_CI_HOME/config.yaml (default ~/.nself/ci/config.yaml).
// Outputs: nil when discovery may run, else an E667 *Error.
// Constraints: user scope only, never a committed file. A missing file means on (the commands are the
//          opt-in); an unreadable, oversized or unparseable file fails closed (E667 config_invalid)
//          because discovery is optional and a broken switch must not turn it on. The path is resolved
//          here instead of through internal/nodes/deployguard so this package imports nothing that can
//          authorize a node (TestImportsCannotAuthorize).

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxConfig = 1 << 20

// userConfig reads both spellings the other user-scope keys accept: nested `ci: {discovery: off}` and the
// flat `ci.discovery: off`. The value is kept as a node so `off`, `false` and `no` all parse whatever
// the YAML version of the decoder.
type userConfig struct {
	CI struct {
		Discovery yaml.Node `yaml:"discovery"`
	} `yaml:"ci"`
	Flat yaml.Node `yaml:"ci.discovery"`
}

// ConfigPath is the user config file under the CI home (home "" means $NSELF_CI_HOME, else ~/.nself/ci).
func ConfigPath(home string) string {
	if home == "" {
		home = os.Getenv("NSELF_CI_HOME")
	}
	if home == "" {
		u, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(u, ".nself", "ci")
	}
	return filepath.Join(home, "config.yaml")
}

// parseSwitch reads an on/off word: off, or invalid for anything unknown (a mapping, a list, a typo). An
// unknown value is not "on": the switch protects privacy, so it fails closed. The same words serve the
// environment variable and the config key.
func parseSwitch(v string) (off, invalid bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "false", "no", "0", "disabled":
		return true, false
	case "on", "true", "yes", "1", "enabled", "", "~", "null":
		return false, false
	}
	return false, true
}

// switchState reads the value of ci.discovery from its YAML node.
func switchState(n yaml.Node) (off, invalid bool) {
	if n.Kind == 0 {
		return false, false // the key is absent
	}
	if n.Kind != yaml.ScalarNode {
		return false, true
	}
	return parseSwitch(n.Value)
}

// Allowed returns nil when discovery may run on this host, else the E667 error naming the switch.
func (g *Guard) Allowed() error {
	getenv := g.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	if off, invalid := parseSwitch(getenv("NSELF_CI_DISCOVERY")); off {
		return disabled(ReasonEnvOff, "LAN discovery is off (NSELF_CI_DISCOVERY=0)")
	} else if invalid {
		return disabled(ReasonEnvOff, "NSELF_CI_DISCOVERY is not on or off, so LAN discovery stays off")
	}
	path := ConfigPath(g.Home)
	if path == "" {
		return nil
	}
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || fi.Size() > maxConfig {
		return disabled(ReasonConfigInvalid, "the user config cannot be read, so LAN discovery stays off")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return disabled(ReasonConfigInvalid, "the user config cannot be read, so LAN discovery stays off")
	}
	var c userConfig
	if err = yaml.Unmarshal(b, &c); err != nil {
		return disabled(ReasonConfigInvalid, "the user config is not valid YAML, so LAN discovery stays off")
	}
	for _, n := range []yaml.Node{c.CI.Discovery, c.Flat} {
		off, invalid := switchState(n)
		if invalid {
			return disabled(ReasonConfigInvalid, "ci.discovery in the user config is not on or off, so LAN discovery stays off")
		}
		if off {
			return disabled(ReasonConfigOff, "LAN discovery is off (ci.discovery: off in config.yaml)")
		}
	}
	return nil
}
