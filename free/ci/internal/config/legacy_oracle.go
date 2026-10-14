package config

// Purpose: tell the user where the legacy runner really behaved differently
// from what a `.ci.yaml` declares, before migration makes the declaration true.
// The legacy runner (internal/legacy/pipeline.go) does not parse YAML: a line
// scanner reads `- name:` at indent 2, `timeout` and `skip_if_env_missing` only
// at indent 2 (where valid YAML never puts them), and list items only at
// indent 4, filing an all-caps or all-digit item under env_required and
// anything else under cmd.
// Inputs:  the raw bytes of a legacy manifest and its YAML-decoded stages.
// Outputs: Notes, one per stage and field where the two readings differ.
// Constraints: the scanner below mirrors pipeline.go (read only, never edited
// here); migration runs what the file DECLARES, and says so wherever the old
// runner ran something else, so no behaviour change is silent.

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// oracleStages returns the stages as the legacy line scanner reads them.
func oracleStages(data []byte) ([]legacyStage, error) {
	stages, _, err := oracleRead(data)
	return stages, err
}

// oracleRead also returns the plugin name the scanner sets: the top-level key
// must read exactly "plugin" (a quoted key or a leading BOM does not), and a
// manifest without it is skipped whole by the legacy runner.
func oracleRead(data []byte) ([]legacyStage, string, error) {
	var stages []legacyStage
	plugin := ""
	cur := -1
	inStages := false
	sc := bufio.NewScanner(bytes.NewReader(data)) // default 64 KiB line limit, as legacy
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		switch {
		case indent == 0:
			// splitYAMLKV: no colon gives key = the whole line and an empty value.
			key, value := trimmed, ""
			if i := strings.Index(trimmed, ":"); i >= 0 {
				key, value = strings.TrimSpace(trimmed[:i]), strings.TrimSpace(trimmed[i+1:])
			}
			inStages, cur = key == "stages", -1
			if key == "plugin" {
				plugin = strings.Trim(value, `"'`)
			}
		case indent == 2 && !inStages:
		case inStages && indent == 2:
			if rest, ok := strings.CutPrefix(trimmed, "- name:"); ok {
				stages = append(stages, legacyStage{Name: strings.Trim(strings.TrimSpace(rest), `"'`)})
				cur = len(stages) - 1
			} else if cur >= 0 {
				k, v, _ := strings.Cut(trimmed, ":")
				switch strings.TrimSpace(k) {
				case "timeout":
					_, _ = fmt.Sscanf(strings.TrimSpace(v), "%d", &stages[cur].Timeout)
				case "skip_if_env_missing":
					stages[cur].SkipIfEnvMissing = strings.TrimSpace(v) == "true"
				}
			}
		case inStages && indent == 4 && cur >= 0:
			if item, ok := strings.CutPrefix(trimmed, "- "); ok {
				item = strings.Trim(strings.TrimSpace(item), `"'`)
				if allCapsOrDigits(item) {
					stages[cur].EnvRequired = append(stages[cur].EnvRequired, item)
				} else {
					stages[cur].Cmd = append(stages[cur].Cmd, item)
				}
			}
		}
	}
	return stages, plugin, sc.Err()
}

func allCapsOrDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// divergenceNotes lists, per stage, what the legacy runner read or did that
// differs from the declared stage the migration will turn into a job.
func divergenceNotes(rel string, data []byte, declared []legacyStage) []string {
	ran, plugin, err := oracleRead(data)
	if err == nil && plugin == "" {
		return []string{rel + ": the legacy runner skipped this whole manifest (it found no top-level plugin: key); the migrated jobs follow the YAML"}
	}
	if err != nil {
		return []string{rel + ": the legacy runner could not read this file (a line over 64 KiB) and skipped the whole manifest; the migrated jobs follow the YAML"}
	}
	if len(ran) != len(declared) {
		return []string{fmt.Sprintf("%s: the legacy line parser found %d stage(s), the YAML declares %d; the migrated jobs follow the YAML", rel, len(ran), len(declared))}
	}
	var out []string
	for i, want := range declared {
		got := ran[i]
		var diffs []string
		if strings.Join(got.Cmd, "\x00") != strings.Join(want.Cmd, "\x00") {
			if len(got.Cmd) == 0 {
				diffs = append(diffs, "read no cmd (it reads list items only at indent 4)")
			} else {
				diffs = append(diffs, fmt.Sprintf("read cmd as %d element(s) instead of %d", len(got.Cmd), len(want.Cmd)))
			}
		}
		if got.Timeout != want.Timeout {
			diffs = append(diffs, fmt.Sprintf("ignored timeout %d (it reads timeout only at indent 2), so the job ran under the 300 s default; the migrated job enforces it", want.Timeout))
		}
		if got.SkipIfEnvMissing != want.SkipIfEnvMissing {
			diffs = append(diffs, "never skipped on a missing variable (it reads skip_if_env_missing only at indent 2), so the stage ran and could fail; the migrated job is optional and is skipped")
		}
		if strings.Join(got.EnvRequired, ",") != strings.Join(want.EnvRequired, ",") {
			diffs = append(diffs, "read env_required differently from the YAML")
		}
		if len(diffs) > 0 {
			out = append(out, printable(fmt.Sprintf("%s: stage %s: the legacy runner %s", rel, want.Name, strings.Join(diffs, "; and "))))
		}
	}
	return out
}

// checkListItems refuses a null or nested cmd / env_required item, which the
// string-list decode would drop or flatten without a word.
func checkListItems(rel string, data []byte) error {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	if bad := findIndirection(&doc); bad != nil {
		return untranslatable(rel, bad.Line, "", "anchors, aliases and merge keys are not supported in a legacy manifest; write the values out")
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "stages" || top.Content[i+1].Kind != yaml.SequenceNode {
			continue
		}
		for si, stage := range top.Content[i+1].Content {
			if stage.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k+1 < len(stage.Content); k += 2 {
				key, list := stage.Content[k].Value, stage.Content[k+1]
				if (key != "cmd" && key != "env_required") || list.Kind != yaml.SequenceNode {
					continue
				}
				for ji, item := range list.Content {
					if item.Kind != yaml.ScalarNode || item.Tag == "!!null" {
						return untranslatable(rel, item.Line, fmt.Sprintf("stages[%d].%s[%d]", si, key, ji), "list items must be plain scalars; a null or nested item would be dropped")
					}
				}
			}
		}
	}
	return nil
}

// findIndirection returns the first anchor, alias or merge key: the typed
// decode would resolve it, and the null check above would not see through it.
func findIndirection(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.AliasNode || n.Anchor != "" || (n.Kind == yaml.ScalarNode && n.Tag == "!!merge") {
		return n
	}
	for _, c := range n.Content {
		if bad := findIndirection(c); bad != nil {
			return bad
		}
	}
	return nil
}
