package internal

// Package internal — pipeline.go
//
// Purpose: Discover .ci.yaml plugin manifests under a search root and run their
//   declared gate stages as named GateResult entries in the CI pipeline.
//   Enables "nself ci run --env staging" to locate E7 plugin stages
//   (plugin-retrieval → plugin-gauth → plugin-clawde-pty) and run in order.
//
// Inputs:  searchRoot string — directory tree to scan (e.g. plugins-pro/)
//          gatewayBase string — resolved gateway base URL ("" = skip routing check)
//          timeout int, verbose bool
// Outputs: []GateResult — one per stage per discovered plugin
// Constraints: Zero external dependencies (no yaml library); simple line parser for
//   .ci.yaml. Integration stages skip if env_required vars absent and
//   skip_if_env_missing=true. Production IP (5.75.235.42) never targeted.
// SPORT: PLUGINS-CI-006

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ciPlugin holds the parsed fields from a .ci.yaml manifest sufficient for
// running gates. Only fields consumed by the pipeline runner are extracted.
type ciPlugin struct {
	Plugin        string
	GatewayRoutes []string
	Stages        []ciStage
}

type ciStage struct {
	Name             string
	Cmd              []string
	Timeout          int
	EnvRequired      []string
	SkipIfEnvMissing bool
}

// DiscoverAndRunPipeline finds all .ci.yaml manifests under searchRoot (up to depth 3)
// and runs their stages in canonical E7 order: plugin-retrieval → plugin-gauth → plugin-clawde-pty.
// If gatewayBase is non-empty, gateway routing checks are appended per plugin.
func DiscoverAndRunPipeline(searchRoot, gatewayBase string, timeout int, verbose bool) ([]GateResult, error) {
	manifests, err := findCIYamls(searchRoot, 3)
	if err != nil {
		return nil, fmt.Errorf("pipeline discovery: %w", err)
	}

	ordered := orderE7Plugins(manifests)

	var results []GateResult
	for _, m := range ordered {
		pluginDir := filepath.Dir(m.path)
		for _, stage := range m.plugin.Stages {
			gr := runCIStage(stage, pluginDir, timeout, verbose)
			gr.Name = m.plugin.Plugin + ":" + gr.Name
			results = append(results, gr)
		}
		if gatewayBase != "" && len(m.plugin.GatewayRoutes) > 0 {
			gwGates := runPluginGatewayRoutes(m.plugin.Plugin, m.plugin.GatewayRoutes, gatewayBase, timeout, verbose)
			results = append(results, gwGates...)
		}
	}
	return results, nil
}

type manifestEntry struct {
	path   string
	plugin ciPlugin
}

// findCIYamls walks dir up to maxDepth levels deep looking for .ci.yaml files.
func findCIYamls(root string, maxDepth int) ([]manifestEntry, error) {
	var found []manifestEntry

	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				walk(filepath.Join(dir, e.Name()), depth+1)
				continue
			}
			if e.Name() != ".ci.yaml" {
				continue
			}
			ciPath := filepath.Join(dir, e.Name())
			p, err := parseCIYaml(ciPath)
			if err != nil || p.Plugin == "" {
				continue
			}
			found = append(found, manifestEntry{path: ciPath, plugin: p})
		}
	}
	walk(root, 0)
	return found, nil
}

// parseCIYaml parses the subset of .ci.yaml fields that the pipeline runner needs.
// Uses a simple line-by-line parser — no external YAML library required.
func parseCIYaml(path string) (ciPlugin, error) {
	f, err := os.Open(path)
	if err != nil {
		return ciPlugin{}, err
	}
	defer f.Close()

	var p ciPlugin
	var currentStage *ciStage
	inStages := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments and blank lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Top-level field: detect indentation level 0 keys.
		indent := len(line) - len(strings.TrimLeft(line, " \t"))

		if indent == 0 {
			inStages = false
			currentStage = nil
			kv := splitYAMLKV(trimmed)
			switch kv[0] {
			case "plugin":
				p.Plugin = strings.Trim(kv[1], `"'`)
			case "stages":
				inStages = true
			case "gateway_routes":
				// Routes follow as list items at indent 2.
				// Handled below when indent == 2 and inStages == false.
			}
			continue
		}

		if indent == 2 && !inStages {
			// gateway_routes items: "  - POST /retrieval"
			if strings.HasPrefix(trimmed, "- ") {
				route := strings.TrimPrefix(trimmed, "- ")
				p.GatewayRoutes = append(p.GatewayRoutes, strings.TrimSpace(route))
			}
			continue
		}

		if inStages && indent == 2 {
			if strings.HasPrefix(trimmed, "- name:") {
				// New stage block.
				name := strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))
				name = strings.Trim(name, `"'`)
				p.Stages = append(p.Stages, ciStage{Name: name})
				currentStage = &p.Stages[len(p.Stages)-1]
			} else if currentStage != nil {
				kv := splitYAMLKV(trimmed)
				switch kv[0] {
				case "timeout":
					var t int
					fmt.Sscanf(kv[1], "%d", &t)
					currentStage.Timeout = t
				case "skip_if_env_missing":
					currentStage.SkipIfEnvMissing = strings.TrimSpace(kv[1]) == "true"
				}
			}
			continue
		}

		if inStages && indent == 4 && currentStage != nil {
			trimmed4 := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed4, "- ") {
				// List item under cmd or env_required.
				// Detect which field we're under by looking at the previous indent-2 key.
				// Simple heuristic: track last indent-2 key name.
				item := strings.TrimSpace(strings.TrimPrefix(trimmed4, "- "))
				item = strings.Trim(item, `"'`)
				// Append to cmd or env_required based on content.
				// Since cmd items are shell tokens and env_required are ALL_CAPS env vars,
				// distinguish by whether item looks like an env var name.
				if isEnvVarName(item) {
					currentStage.EnvRequired = append(currentStage.EnvRequired, item)
				} else {
					currentStage.Cmd = append(currentStage.Cmd, item)
				}
			}
		}
	}
	return p, scanner.Err()
}

// splitYAMLKV splits "key: value" → ["key", "value"]. Returns ["", ""] on no colon.
func splitYAMLKV(s string) [2]string {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return [2]string{s, ""}
	}
	return [2]string{strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+1:])}
}

// isEnvVarName returns true if s looks like an environment variable name (UPPER_CASE).
func isEnvVarName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// orderE7Plugins sorts manifests so E7 plugins run in canonical order:
// plugin-retrieval → plugin-gauth → plugin-clawde-pty. Others appended last.
func orderE7Plugins(manifests []manifestEntry) []manifestEntry {
	e7order := []string{"plugin-retrieval", "plugin-gauth", "plugin-clawde-pty"}
	pos := make(map[string]int, len(e7order))
	for i, name := range e7order {
		pos[name] = i
	}

	buckets := make([][]manifestEntry, len(e7order))
	var tail []manifestEntry
	for _, m := range manifests {
		if i, ok := pos[m.plugin.Plugin]; ok {
			buckets[i] = append(buckets[i], m)
		} else {
			tail = append(tail, m)
		}
	}

	ordered := make([]manifestEntry, 0, len(manifests))
	for _, b := range buckets {
		ordered = append(ordered, b...)
	}
	return append(ordered, tail...)
}

// runCIStage executes a single declared stage, skipping if env vars are absent.
func runCIStage(stage ciStage, workDir string, defaultTimeout int, verbose bool) GateResult {
	if stage.SkipIfEnvMissing {
		for _, envVar := range stage.EnvRequired {
			if os.Getenv(envVar) == "" {
				return GateResult{
					Name:   stage.Name,
					Passed: true,
					Output: fmt.Sprintf("skipped: %s not set", envVar),
				}
			}
		}
	}

	if len(stage.Cmd) == 0 {
		return GateResult{Name: stage.Name, Passed: false, Output: "no cmd declared in .ci.yaml"}
	}

	t := stage.Timeout
	if t <= 0 {
		t = defaultTimeout
	}

	return runStep(stage.Name, workDir, t, verbose, stage.Cmd[0], stage.Cmd[1:]...)
}

// runPluginGatewayRoutes runs routing checks for a plugin's declared gateway_routes.
// Accepts HTTP 2xx–4xx (route reachable + routing live); rejects 5xx + connection failures.
func runPluginGatewayRoutes(plugin string, routes []string, gatewayBase string, timeout int, verbose bool) []GateResult {
	tmpDir := os.TempDir()
	var results []GateResult

	for _, route := range routes {
		parts := strings.SplitN(strings.TrimSpace(route), " ", 2)
		method, path := "GET", route
		if len(parts) == 2 {
			method, path = parts[0], parts[1]
		}

		url := gatewayBase + path
		name := fmt.Sprintf("gateway[%s]:%s", plugin, path)
		args := []string{
			"-sf",
			"--max-time", fmt.Sprintf("%d", timeout),
			"-X", method,
			"-H", "Content-Type: application/json",
			"-d", "{}",
			"-o", "/dev/null",
			"-w", "%{http_code}",
			url,
		}

		gr := runStep(name, tmpDir, timeout, verbose, "curl", args...)
		if !gr.Passed && len(gr.Output) == 3 {
			code := gr.Output
			if code >= "200" && code < "500" {
				gr.Passed = true
			}
		}
		results = append(results, gr)
	}
	return results
}
