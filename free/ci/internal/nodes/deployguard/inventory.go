package deployguard

// Purpose: read one project's deploy inventory through `nself deploy targets --json`
// (contract:cli.deploy-targets) and turn it into exclusion entries.
// Inputs:  a project root and a RunFunc (production: the exec funnel, see runner.go).
// Outputs: []Entry (host, address, fingerprint) from every target row.
// Constraints: `nself help --json deploy targets` runs first and must report side_effect "read" for that
//          command, else nothing else runs (refusal, not a guess); the nself output is parsed, never
//          executed; the error text never carries stderr; no connection to a deploy host is made here or
//          by the command (its own contract, D14).

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// RunFunc runs argv with dir as the project root and returns its combined output and exit code.
type RunFunc func(ctx context.Context, dir string, argv ...string) (out []byte, exit int, err error)

// Reader reads deploy inventories.
type Reader struct {
	Run RunFunc
	// Bin is the nself binary name or path; "nself" when empty.
	Bin string
}

func (r Reader) bin() string {
	if r.Bin == "" {
		return "nself"
	}
	return r.Bin
}

// Read returns the entries of the project at root.
func (r Reader) Read(ctx context.Context, root string) ([]Entry, error) {
	if r.Run == nil {
		return nil, newErr(ReasonInventory, "no command runner for the deploy inventory")
	}
	out, exit, err := r.Run(ctx, root, r.bin(), "help", "--json", "deploy", "targets")
	if err != nil || exit != 0 {
		return nil, newErr(ReasonInventory, "nself help --json deploy targets failed (exit %d)", exit)
	}
	if !sideEffectIsRead(out) {
		return nil, newErr(ReasonSideEffect, "nself reports deploy targets as something other than a read command; nothing was run")
	}
	out, exit, err = r.Run(ctx, root, r.bin(), "deploy", "targets", "--json")
	if err != nil || exit != 0 {
		return nil, newErr(ReasonInventory, "nself deploy targets --json failed (exit %d)", exit)
	}
	return parseTargets(out)
}

// sideEffectIsRead is true only when every help entry for `deploy targets` says "read" and one exists.
func sideEffectIsRead(out []byte) bool {
	doc, ok := firstJSON(out, func(m map[string]any) bool { return m["data"] != nil })
	if !ok {
		return false
	}
	found, bad := 0, false
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			if p, _ := t["path"].(string); strings.Join(strings.Fields(p), " ") == "deploy targets" {
				found++
				if se, _ := t["side_effect"].(string); se != "read" {
					bad = true
				}
			}
			for _, c := range t {
				walk(c)
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(doc["data"])
	return found > 0 && !bad
}

type targetsDoc struct {
	Data struct {
		Targets []struct {
			Host                string   `json:"host"`
			SSHHostname         string   `json:"ssh_hostname"`
			Addresses           []string `json:"addresses"`
			HostKeyFingerprints []string `json:"host_key_fingerprints"`
		} `json:"targets"`
	} `json:"data"`
}

func parseTargets(out []byte) ([]Entry, error) {
	raw, ok := firstJSON(out, func(m map[string]any) bool { return m["command"] == "deploy targets" })
	if !ok {
		return nil, newErr(ReasonInventory, "nself deploy targets --json printed no deploy targets document")
	}
	b, _ := json.Marshal(raw)
	var d targetsDoc
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, newErr(ReasonInventory, "nself deploy targets --json document does not match contract:cli.deploy-targets")
	}
	if _, ok := raw["data"].(map[string]any)["targets"]; !ok {
		return nil, newErr(ReasonInventory, "nself deploy targets --json document has no targets list")
	}
	var es []Entry
	for _, t := range d.Data.Targets {
		for _, h := range append([]string{t.Host, t.SSHHostname}, t.Addresses...) {
			if h != "" {
				es = append(es, Entry{KindHost, h, SourceInventory})
			}
		}
		for _, f := range t.HostKeyFingerprints {
			es = append(es, Entry{KindFingerprint, f, SourceInventory})
		}
	}
	return es, nil
}

// firstJSON decodes the first JSON object in out (stderr noise may precede it) that accept approves.
func firstJSON(out []byte, accept func(map[string]any) bool) (map[string]any, bool) {
	for i := 0; i < len(out); i++ {
		if out[i] != '{' || (i > 0 && out[i-1] != '\n') {
			continue
		}
		var m map[string]any
		if err := json.NewDecoder(bytes.NewReader(out[i:])).Decode(&m); err == nil && accept(m) {
			return m, true
		}
	}
	return nil, false
}
