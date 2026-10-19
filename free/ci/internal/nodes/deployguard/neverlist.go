package deployguard

// Purpose: the operator's never-list: hosts that must never run CI whatever any inventory says, from user
// scope config only (`ci.never_hosts` in $NSELF_CI_HOME/config.yaml or ~/.nself/ci/config.yaml), plus the
// optional `ci.project_roots` list of projects whose inventories to read.
// Inputs:  the config file path.
// Outputs: []Entry with source never-list; the project roots.
// Constraints: user scope only, never a committed file; an entry is a host, an address, a CIDR prefix,
//          `SHA256:<fingerprint>` or `machine-id:<id>`; a missing file is an empty list, an unparseable
//          file is an error (the caller keeps the cached inventory and warns); a file over 1 MiB is refused.

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type userConfig struct {
	CI struct {
		NeverHosts   []string `yaml:"never_hosts"`
		ProjectRoots []string `yaml:"project_roots"`
	} `yaml:"ci"`
	FlatNever []string `yaml:"ci.never_hosts"`
	FlatRoots []string `yaml:"ci.project_roots"`
}

// ConfigPath is the user config file under the CI home.
func ConfigPath(home string) string {
	if home == "" {
		if v := os.Getenv("NSELF_CI_HOME"); v != "" {
			home = v
		} else if u, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(u, ".nself", "ci")
		}
	}
	return filepath.Join(home, "config.yaml")
}

func loadConfig(path string) (userConfig, error) {
	var c userConfig
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if fi.Size() > 1<<20 {
		return c, newErr(ReasonInventory, "%s is larger than 1 MiB", filepath.Base(path))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = yaml.Unmarshal(b, &c); err != nil {
		return c, newErr(ReasonInventory, "%s is not valid YAML", filepath.Base(path))
	}
	return c, nil
}

// LoadNeverList returns the never-list entries of the config at path.
func LoadNeverList(path string) ([]Entry, error) {
	c, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	var es []Entry
	for _, v := range append(c.CI.NeverHosts, c.FlatNever...) {
		if e, ok := neverEntry(v); ok {
			es = append(es, e)
		}
	}
	return es, nil
}

// LoadProjectRoots returns the `ci.project_roots` of the config at path (absolute paths only).
func LoadProjectRoots(path string) ([]string, error) {
	c, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range append(c.CI.ProjectRoots, c.FlatRoots...) {
		if r = strings.TrimSpace(r); filepath.IsAbs(r) {
			out = append(out, filepath.Clean(r))
		}
	}
	return out, nil
}

func neverEntry(v string) (Entry, bool) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return Entry{}, false
	case strings.HasPrefix(strings.ToLower(v), "machine-id:"):
		return Entry{KindMachineID, v[len("machine-id:"):], SourceNeverList}, true
	case NormalizeFingerprint(v) != "":
		return Entry{KindFingerprint, v, SourceNeverList}, true
	}
	if _, err := netip.ParsePrefix(v); err == nil {
		return Entry{KindPrefix, v, SourceNeverList}, true
	}
	return Entry{KindHost, v, SourceNeverList}, true
}
