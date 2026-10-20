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
	"errors"
	"io"
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

// maxFile caps every file the guard reads from the host (config, machine ids, host keys).
const maxFile = 1 << 20

// readBounded reads a regular file of at most maxFile bytes. A FIFO or device is refused before the open (an
// open or read would block or never end), the open handle is checked again, and a file that grows after the
// check is still cut off by the read bound.
func readBounded(path string) ([]byte, error) {
	if fi, err := os.Stat(path); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	b, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxFile {
		return nil, os.ErrInvalid
	}
	return b, nil
}

func loadConfig(path string) (userConfig, error) {
	var c userConfig
	b, err := readBounded(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if errors.Is(err, os.ErrInvalid) {
		return c, newErr(ReasonInventory, "%s is not a regular file of at most 1 MiB", filepath.Base(path))
	}
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
		e, ok, err := neverEntry(v)
		if err != nil {
			return nil, err
		}
		if ok {
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

func neverEntry(v string) (Entry, bool, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return Entry{}, false, nil
	case strings.HasPrefix(strings.ToLower(v), "machine-id:"):
		return Entry{KindMachineID, v[len("machine-id:"):], SourceNeverList}, true, nil
	case strings.HasPrefix(strings.ToUpper(v), "SHA256:") && NormalizeFingerprint(v) != "":
		return Entry{KindFingerprint, v, SourceNeverList}, true, nil
	}
	if _, err := netip.ParsePrefix(v); err == nil {
		return Entry{KindPrefix, v, SourceNeverList}, true, nil
	}
	if NormalizeFingerprint(v) != "" && !strings.Contains(v, ".") {
		return Entry{}, false, newErr(ReasonInventory, "never_hosts entry %q looks like a host-key fingerprint; write it as SHA256:<fingerprint>", v)
	}
	if strings.Contains(v, "/") {
		return Entry{}, false, newErr(ReasonInventory, "never_hosts entry %q is not a valid CIDR prefix", v)
	}
	return Entry{KindHost, v, SourceNeverList}, true, nil
}
