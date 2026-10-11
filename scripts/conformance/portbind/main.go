// Command portbind checks that every published port of every compose service binds 127.0.0.1.
//
// Usage: portbind [-require FILE]... FILE...
// Prints one line per violation ("<file>: service <svc> <description>") and exits 0 when there is none,
// 1 when there is at least one, 2 on a usage or I/O error. A -require file must parse and hold a top-level
// `services:` mapping, otherwise the tool exits 2 (fail closed). Other files that do not parse as YAML or
// have no top-level `services` mapping (k8s manifests, workflows, Helm templates) are skipped silently.
// Used by scripts/conformance/rules/port-bind.sh (rule port-bind).
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run is the testable entry point.
func run(args []string, stdout, stderr io.Writer) int {
	var required, files []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-require" || a == "--require":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "portbind: -require needs a file")
				return 2
			}
			i++
			required = append(required, args[i])
		case strings.HasPrefix(a, "-") && a != "-":
			fmt.Fprintf(stderr, "portbind: unknown flag %s\n", a)
			return 2
		default:
			files = append(files, a)
		}
	}
	if len(required) == 0 && len(files) == 0 {
		fmt.Fprintln(stderr, "usage: portbind [-require FILE]... FILE...")
		return 2
	}
	// One check per file: a required file wins over the same path given as a plain file.
	seen := map[string]bool{}
	type job struct {
		path string
		req  bool
	}
	var jobs []job
	for _, f := range required {
		if k := filepath.Clean(f); !seen[k] {
			seen[k] = true
			jobs = append(jobs, job{f, true})
		}
	}
	for _, f := range files {
		if k := filepath.Clean(f); !seen[k] {
			seen[k] = true
			jobs = append(jobs, job{f, false})
		}
	}
	violations := 0
	for _, j := range jobs {
		data, err := os.ReadFile(j.path)
		if err != nil {
			fmt.Fprintf(stderr, "portbind: %v\n", err)
			return 2
		}
		vs, hasServices, perr := checkFile(data, j.req)
		if j.req && (perr != nil || !hasServices) {
			if perr == nil {
				perr = errors.New("no top-level services mapping")
			}
			fmt.Fprintf(stderr, "portbind: required file %s: %v\n", j.path, perr)
			return 2
		}
		for _, v := range vs {
			fmt.Fprintf(stdout, "%s: %s\n", j.path, v)
			violations++
		}
	}
	if violations > 0 {
		return 1
	}
	return 0
}

// checkFile decodes every YAML document in data and returns the violations of the documents that hold a
// top-level services mapping, whether any did, and the first parse error (documents before it are kept).
// strict is true for a -require file. A long-syntax ports entry without `target` is not valid compose (the
// compose spec requires target); outside strict mode it is skipped, because Helm values files also have a
// top-level `services` mapping whose `ports` entries look like {port: 80, protocol: TCP}.
func checkFile(data []byte, strict bool) (vs []string, hasServices bool, err error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc any
		if derr := dec.Decode(&doc); derr != nil {
			if !errors.Is(derr, io.EOF) {
				err = derr
			}
			return vs, hasServices, err
		}
		root, ok := doc.(map[string]any)
		if !ok {
			continue
		}
		svcs, ok := root["services"].(map[string]any)
		if !ok {
			continue
		}
		hasServices = true
		names := make([]string, 0, len(svcs))
		for n := range svcs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			for _, d := range checkService(svcs[n], strict) {
				vs = append(vs, fmt.Sprintf("service %s %s", n, d))
			}
		}
	}
}

// checkService returns one description per violation of a single service definition.
func checkService(def any, strict bool) []string {
	if def == nil {
		return nil
	}
	svc, ok := def.(map[string]any)
	if !ok {
		return []string{"is not a mapping (cannot verify its ports)"}
	}
	var out []string
	if nm, ok := svc["network_mode"].(string); ok && strings.TrimSpace(nm) == "host" {
		out = append(out, "uses network_mode host, which exposes every listening port")
	}
	ports, present := svc["ports"]
	if !present || ports == nil {
		return out
	}
	list, ok := ports.([]any)
	if !ok {
		return append(out, "has a ports value that is not a list (cannot verify)")
	}
	for _, p := range list {
		if d := checkPort(p, strict); d != "" {
			out = append(out, d)
		}
	}
	return out
}

// checkPort returns the violation description for one ports entry, or "" when it binds 127.0.0.1.
func checkPort(p any, strict bool) string {
	switch v := p.(type) {
	case string:
		return checkShort(v)
	case int:
		return checkShort(fmt.Sprint(v))
	case map[string]any:
		if _, ok := v["target"]; !ok && !strict {
			return ""
		}
		return checkLong(v)
	default:
		return fmt.Sprintf("has a ports entry %v of a type that cannot be verified", p)
	}
}

func checkLong(m map[string]any) string {
	desc := fmt.Sprintf("publishes target %v", m["target"])
	if pub, ok := m["published"]; ok {
		desc += fmt.Sprintf(" published %v", pub)
	}
	hip, present := m["host_ip"]
	if !present || hip == nil {
		return desc + " on all interfaces (no host_ip)"
	}
	s, ok := hip.(string)
	if !ok {
		return desc + fmt.Sprintf(" with host_ip %v, which is not a literal 127.0.0.1", hip)
	}
	if s == "127.0.0.1" {
		return ""
	}
	if strings.Contains(s, "${") {
		return desc + fmt.Sprintf(" with host_ip %q, which is not a literal 127.0.0.1", s)
	}
	return desc + " on " + s
}

func checkShort(raw string) string {
	spec := strings.TrimSpace(raw)
	for _, suf := range []string{"/tcp", "/udp"} {
		spec = strings.TrimSuffix(spec, suf)
	}
	parts := splitPorts(spec)
	switch len(parts) {
	case 1, 2:
		return fmt.Sprintf("publishes %q on all interfaces", raw)
	case 3:
		hip := strings.TrimSuffix(strings.TrimPrefix(parts[0], "["), "]")
		if hip == "127.0.0.1" {
			return ""
		}
		if strings.Contains(hip, "${") {
			return fmt.Sprintf("publishes %q with a host IP that is not a literal 127.0.0.1", raw)
		}
		if hip == "" {
			return fmt.Sprintf("publishes %q on all interfaces", raw)
		}
		return fmt.Sprintf("publishes %q on %s", raw, hip)
	default:
		return fmt.Sprintf("has a port mapping %q that cannot be verified", raw)
	}
}

// splitPorts splits on ':' outside ${...} (nesting counted) and outside [...] (IPv6 host IPs).
func splitPorts(s string) []string {
	var parts []string
	depth, bracket, start := 0, false, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '$' && i+1 < len(s) && s[i+1] == '{':
			depth++
			i++
		case c == '}' && depth > 0:
			depth--
		case c == '[' && depth == 0:
			bracket = true
		case c == ']' && depth == 0:
			bracket = false
		case c == ':' && depth == 0 && !bracket:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}
