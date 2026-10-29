package main

// Purpose: render the `commands` block of free/ci/plugin.json from a Spec and splice it into the manifest.
// Inputs:  a Spec and the current plugin.json bytes.
// Outputs: the new plugin.json bytes: only the `commands` key changes; keys are sorted, 2-space indent, trailing newline.
// Constraints: root args and flags stay in the spec only (manifest v2 roots declare none); the root summary feeds the v1
//              compatibility key cliCommands[0].description, so it must equal it; deterministic (same inputs, same bytes); no HTML escaping; rows are ordered by path.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// toMap converts any JSON-encodable value to generic maps so encoding sorts the keys.
func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}

// Commands builds the `commands` object for a spec.
func Commands(s *Spec) (map[string]any, error) {
	root, err := toMap(s.Root)
	if err != nil {
		return nil, err
	}
	root["binary"], root["command"] = s.Binary, s.Command
	// The manifest v2 root declares no args or flags (cli manifestv2: the root has none); the spec keeps them for the tests.
	delete(root, "args")
	delete(root, "flags")
	rows := append([]Row(nil), s.Commands...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Path < rows[j].Path })
	subs := make([]any, 0, len(rows))
	for _, r := range rows {
		m, err := toMap(r)
		if err != nil {
			return nil, err
		}
		subs = append(subs, m)
	}
	root["subcommands"] = subs
	return root, nil
}

// Render returns manifest with its `commands` key replaced by the block generated from s.
func Render(s *Spec, manifest []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(manifest, &doc); err != nil {
		return nil, fmt.Errorf("plugin.json: %w", err)
	}
	cmds, err := Commands(s)
	if err != nil {
		return nil, err
	}
	doc["commands"] = cmds
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
