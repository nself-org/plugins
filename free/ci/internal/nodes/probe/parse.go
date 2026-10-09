package probe

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var versionRE = regexp.MustCompile(`\d+(?:\.\d+)*(?:[A-Za-z0-9.+-]*)`)

func parseUname(s string) (os, arch, kernel *string) {
	f := strings.Fields(s)
	if len(f) != 3 || strings.ContainsAny(s, "\x00\r") {
		return
	}
	switch strings.ToLower(f[0]) {
	case "linux":
		os = ptr("linux")
	case "darwin":
		os = ptr("darwin")
	default:
		return
	}
	if !versionRE.MatchString(f[1]) {
		return os, nil, nil
	}
	kernel = &f[1]
	switch f[2] {
	case "x86_64", "amd64":
		arch = ptr("amd64")
	case "aarch64", "arm64":
		arch = ptr("arm64")
	case "armv7l":
		arch = ptr("arm")
	default:
		arch = nil
	}
	return
}

func ptr[T any](v T) *T { return &v }

func parseOSRelease(s string) *string {
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "VERSION_ID=") {
			continue
		}
		v := strings.TrimPrefix(line, "VERSION_ID=")
		if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		if versionRE.MatchString(v) {
			return &v
		}
	}
	return nil
}

func parseNproc(s string) *float64 {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 1_000_000 {
		return nil
	}
	return ptr(float64(n))
}

func parseDisk(s string) *int64 {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "Available") {
		return nil
	}
	f := strings.Fields(lines[1])
	if len(f) < 6 || f[len(f)-1] != "/" {
		return nil
	}
	kib, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil || kib < 0 {
		return nil
	}
	return ptr(kib / 1024)
}

func parseTools(s string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		name := strings.TrimSpace(line)
		if !strings.HasPrefix(name, "/") {
			continue
		}
		name = name[strings.LastIndex(name, "/")+1:]
		switch name {
		case "docker", "podman", "runsc", "tart", "go", "node", "pnpm", "cargo", "rustc":
			out[name] = true
		}
	}
	return out
}

func parseVersion(s string) *string {
	s = strings.TrimSpace(s)
	if len(s) > 256 || strings.ContainsRune(s, '\x00') {
		return nil
	}
	v := versionRE.FindString(s)
	if v == "" {
		return nil
	}
	return &v
}

func parseXcode(version, path string) *string {
	if !strings.HasPrefix(strings.TrimSpace(version), "Xcode ") || !strings.HasPrefix(strings.TrimSpace(path), "/") {
		return nil
	}
	return parseVersion(version)
}

func parseNvidia(s string) *[]string {
	if strings.TrimSpace(s) == "" || strings.TrimSpace(s) == "No devices were found" {
		return ptr([]string{})
	}
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.HasPrefix(line, "GPU ") && strings.Contains(line, ":") {
			return ptr([]string{"gpu"})
		}
	}
	return nil
}

func parseDisplays(s string) *[]string {
	var doc struct {
		Displays []json.RawMessage `json:"SPDisplaysDataType"`
	}
	if json.Unmarshal([]byte(s), &doc) != nil || doc.Displays == nil {
		return nil
	}
	if len(doc.Displays) == 0 {
		return ptr([]string{})
	}
	return ptr([]string{"gpu"})
}

func applyToolFacts(c *model.Capability, outputs map[string]string, osName string, now time.Time) {
	paths := parseTools(outputs[cmdTools])
	_, toolsObserved := outputs[cmdTools]
	c.Tools.Docker = observed[bool](nil, now)
	if paths["docker"] && parseVersion(outputs[cmdDocker]) != nil {
		c.Tools.Docker = value(true, now)
	} else if toolsObserved && !paths["docker"] {
		c.Tools.Docker = value(false, now)
	}
	c.Tools.Podman = observed[bool](nil, now)
	c.Tools.GVisor = observed[bool](nil, now)
	c.Tools.Tart = observed[bool](nil, now)
	if toolsObserved {
		c.Tools.Podman = value(paths["podman"], now)
		c.Tools.GVisor = value(paths["runsc"], now)
		c.Tools.Tart = value(paths["tart"], now)
	}
	if osName == "darwin" && parseVersion(outputs[cmdTart]) == nil {
		c.Tools.Tart = observed[bool](nil, now)
	}
	toolchains := make(map[string]model.Fact[string], len(c.Tools.Toolchains)+8)
	for name, fact := range c.Tools.Toolchains {
		toolchains[name] = fact
	}
	c.Tools.Toolchains = toolchains
	for _, name := range []string{"go", "node", "pnpm", "cargo", "rustc"} {
		// A path proves presence; the fixed argv set does not reveal the version.
		if paths[name] {
			c.Tools.Toolchains[name] = value("present", now)
		} else {
			c.Tools.Toolchains[name] = observed[string](nil, now)
		}
	}
	for name, command := range map[string]string{"python3": cmdPython, "swift": cmdSwift} {
		if name == "swift" && osName != "darwin" {
			continue
		}
		c.Tools.Toolchains[name] = observed(parseVersion(outputs[command]), now)
	}
	if osName == "darwin" {
		c.Tools.Toolchains["xcode"] = observed(parseXcode(outputs[cmdXcode], outputs[cmdXcodePath]), now)
	}
	c.Tools.Browsers = observed[[]string](nil, now)
	c.Resources.Accelerators = observed[[]string](nil, now)
	if osName == "linux" {
		if output, ran := outputs[cmdNvidia]; ran {
			c.Resources.Accelerators = observed(parseNvidia(output), now)
		}
	}
	if osName == "darwin" {
		if output, ran := outputs[cmdDisplays]; ran {
			c.Resources.Accelerators = observed(parseDisplays(output), now)
		}
	}
}
