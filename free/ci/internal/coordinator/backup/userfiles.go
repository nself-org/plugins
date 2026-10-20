package backup

// Purpose: what a restore keeps of the user files of the dir it replaces (D-1308): a never_hosts entry added
//          after the backup must survive the swap, because a missing never-host is a safety loss.
// Inputs:  the archived file and the replaced dir's copy of it.
// Outputs: the bytes to install; the replaced dir's copies are read once, before the swap, and kept in the
//          restore marker so a re-run merges the same inputs.
// Constraints: config.yaml keeps the archived bytes unless the replaced file holds a never_hosts entry the
//          archive lacks (then the file is re-encoded with the union; comments are lost); known_hosts is the
//          archive's file as before (the replaced copy stays as known_hosts.pre-restore-<ts>); an unreadable
//          file is ignored.

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// readUserFiles returns the replaced dir's config.yaml, keyed by file name (the only file that is merged).
func readUserFiles(dataDir string) map[string]string {
	out := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(dataDir, "config.yaml")); err == nil {
		out["config.yaml"] = string(b)
	}
	return out
}

// mergeUserFile returns what to install for the archived file name given the replaced dir's copy.
func mergeUserFile(name string, archived []byte, replaced string) []byte {
	switch name {
	case "config.yaml":
		return mergeConfig(archived, []byte(replaced))
	}
	return archived
}

// neverHosts reads ci.never_hosts (nested and flat spelling) from a config.yaml.
func neverHosts(b []byte) (list []string, doc map[string]any, ok bool) {
	if len(strings.TrimSpace(string(b))) == 0 || yaml.Unmarshal(b, &doc) != nil {
		return nil, nil, false
	}
	if ci, isMap := doc["ci"].(map[string]any); isMap {
		list = append(list, anyStrings(ci["never_hosts"])...)
	}
	list = append(list, anyStrings(doc["ci.never_hosts"])...)
	return list, doc, true
}

func anyStrings(v any) []string {
	var out []string
	if l, isList := v.([]any); isList {
		for _, e := range l {
			if s, isStr := e.(string); isStr {
				out = append(out, s)
			}
		}
	}
	return out
}

// mergeConfig adds the replaced file's never_hosts entries the archived file lacks.
func mergeConfig(archived, replaced []byte) []byte {
	extra, _, ok := neverHosts(replaced)
	if !ok {
		return archived
	}
	have, doc, ok := neverHosts(archived)
	if !ok {
		doc = map[string]any{}
	}
	seen := map[string]bool{}
	for _, h := range have {
		seen[h] = true
	}
	var add []string
	for _, h := range extra {
		if !seen[h] {
			seen[h] = true
			add = append(add, h)
		}
	}
	if len(add) == 0 {
		return archived
	}
	all := append(have, add...)
	merged := make([]any, len(all))
	for i, h := range all {
		merged[i] = h
	}
	if _, flat := doc["ci.never_hosts"]; flat {
		doc["ci.never_hosts"] = merged
	} else {
		ci, isMap := doc["ci"].(map[string]any)
		if !isMap {
			ci = map[string]any{}
			doc["ci"] = ci
		}
		ci["never_hosts"] = merged
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return archived
	}
	return out
}
