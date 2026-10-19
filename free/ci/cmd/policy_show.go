package main

// Purpose: `nself ci policy show [--effective] [--json]` (P7-TRUST-26, Epic D15):
// the effective policy for this repository with the source of every key, the
// pinned commit, the fetch state, the digest and the fields a scope was not
// allowed to set (a Spend key in the branch or revision, a widening revision).
// Inputs:  the repository root, the CI store (operator rows and pin) and git.
// Outputs: a table or one v1 JSON envelope whose data is {effective, sources,
// pinned_commit, fetch, digest, ignored: [{key, source, code, reason}]}.
// Constraints: read only; a Secret key prints as "[REDACTED]"; text is printed
// through plainText so a branch value cannot forge lines or terminal escapes.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type ignoredJSON struct {
	Key    string `json:"key"`
	Source string `json:"source"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type showJSON struct {
	Effective    map[string]any    `json:"effective"`
	Sources      map[string]string `json:"sources"`
	PinnedCommit string            `json:"pinned_commit"`
	Fetch        string            `json:"fetch"`
	Digest       string            `json:"digest"`
	Ignored      []ignoredJSON     `json:"ignored"`
}

func showPolicy(ctx context.Context, env policyEnv, st *store.Store, root, project string, f policyFlags) int {
	const command = "ci policy show"
	res := policy.Resolver{Store: st, Run: env.Run}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: project, Team: f.team, Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return policyFail(env, command, f.json, err)
	}
	data := showJSON{Effective: map[string]any{}, Sources: map[string]string{}, PinnedCommit: got.PinnedCommit, Fetch: string(got.Fetch), Digest: string(got.Digest), Ignored: []ignoredJSON{}}
	for k, v := range got.Values {
		data.Effective[k] = v
		if spec, ok := policy.Default.Find(k); ok && spec.Secret {
			data.Effective[k] = policy.Masked
		}
		data.Sources[k] = string(got.Sources[k])
	}
	for _, ig := range got.Ignored {
		code := ""
		if m := leadingCode.FindStringSubmatch(ig.Reason); m != nil {
			code = m[1]
		}
		data.Ignored = append(data.Ignored, ignoredJSON{Key: ig.Key, Source: string(ig.Scope), Code: code, Reason: strings.TrimPrefix(ig.Reason, code+" ")})
	}
	if f.json {
		return adminWriteData(env.adminEnv, command, data)
	}
	printPolicy(env, data, f.effective)
	return 0
}

func printPolicy(env policyEnv, d showJSON, effectiveOnly bool) {
	keys := make([]string, 0, len(d.Effective))
	for k := range d.Effective {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := json.Marshal(d.Effective[k])
		_, _ = fmt.Fprintf(env.Out, "%s\t%s\t%s\n", plainText(k), plainText(string(v)), d.Sources[k])
	}
	if effectiveOnly {
		return
	}
	_, _ = fmt.Fprintf(env.Out, "\npinned_commit: %s\nfetch: %s\ndigest: %s\n", d.PinnedCommit, d.Fetch, d.Digest)
	for _, ig := range d.Ignored {
		_, _ = fmt.Fprintf(env.Out, "ignored\t%s\t%s\t%s %s\n", plainText(ig.Key), ig.Source, ig.Code, plainText(ig.Reason))
	}
}
