package main

// Purpose: `nself ci artifacts --cache list|show|prune|verify [--run <run>]
// [--json]` (P7-CACHE-02, Epic D5/D20): the views over cache entries and their
// per-attempt usage rows, installed through P7-CACHE-03's `--cache` slot. This
// file holds the slot, the shared environment, list and show; verify and prune
// are in artifacts_cache_ops.go.
// Inputs:  argv after `--cache`; env NSELF_CI_HOME, NSELF_CI_REPO (the viewer's
// project), NSELF_CI_ROLE; the cache caps NSELF_CI_CACHE_MAX_BYTES and
// NSELF_CI_CACHE_MAX_AGE.
// Outputs: a table, key/value lines or one v1 JSON envelope; exit 0, 1 (usage,
// unknown run, corrupt entries found by verify) or 2 (infrastructure).
// Constraints: V15 only. Side-effect classes: list, show and verify read (verify
// re-hashes every blob and drops nothing), prune deletes and needs the admin
// role because the byte cap is store wide. Only the viewer's project entries are
// listed. No env value or secret is ever stored in what these views print.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

func init() { registerArtifactsCache(artifactsCacheMain) }

const cacheUsage = "usage: nself ci artifacts --cache list [--run R] | show --run R | show <key> | prune [--dry-run] | verify [--run R]  (all take --json)"

func artifactsCacheMain(args []string) int {
	subs := map[string]func([]string) int{"list": cacheList, "show": cacheShow, "prune": cachePrune, "verify": cacheVerify}
	if len(args) == 0 || subs[args[0]] == nil {
		fmt.Fprintln(os.Stderr, cacheUsage)
		return 1
	}
	return subs[args[0]](args[1:])
}

// cacheEntryView is one entry as printed; it carries digests and flags only.
type cacheEntryView struct {
	Key             string `json:"key"`
	Spec            string `json:"spec"`
	Trust           string `json:"trust"`
	Protection      string `json:"protection"`
	Bytes           int64  `json:"bytes"`
	Blobs           int    `json:"blobs"`
	Attempt         string `json:"attempt"`
	SecretsReceived bool   `json:"secrets_received"`
	PrivateDeps     bool   `json:"private_deps"`
	ScannedClean    bool   `json:"scanned_clean"`
	CreatedAt       string `json:"created_at"`
	LastUsedAt      string `json:"last_used_at"`
}

func viewOf(m entries.Manifest) cacheEntryView {
	v := cacheEntryView{Key: m.Key, Spec: m.Spec, Bytes: m.Bytes(), Blobs: len(m.Blobs), Attempt: m.Writer.AttemptID, SecretsReceived: m.Writer.SecretsReceived,
		PrivateDeps: m.Writer.PrivateDeps, ScannedClean: m.Writer.ScannedClean, CreatedAt: time.Unix(0, m.CreatedAt).UTC().Format(time.RFC3339),
		LastUsedAt: time.Unix(0, m.LastUsedAt).UTC().Format(time.RFC3339)}
	if ns, err := authz.DecodeNamespace(m.Namespace); err == nil {
		v.Trust, v.Protection = string(ns.Trust), string(ns.Protection)
	}
	return v
}

// cacheEnv is artifactsEnv plus the entries store.
type cacheEnv struct {
	*artifactsEnv
	es *entries.Store
}

func openCache(fs *flag.FlagSet) (*cacheEnv, error) {
	env, err := openArtifacts(fs)
	if err != nil {
		return nil, err
	}
	return &cacheEnv{artifactsEnv: env, es: entries.New(env.st, env.blobs, entries.Options{Home: records.StoreDir(os.Getenv("NSELF_CI_HOME"))})}, nil
}

// runAttempts returns the attempt ids of run, or an E713 error for an unknown run.
func (e *cacheEnv) runAttempts(ctx context.Context, run string) (map[string]bool, error) {
	// a run of another project reads exactly like a run that does not exist
	if p, ok, err := e.st.PipelineByID(ctx, run); err != nil {
		return nil, err
	} else if !ok || p.Project != e.viewer.Project {
		return nil, &records.Error{ID: records.CodeNotFound, Reason: "not_found", Msg: "no such run"}
	}
	list, err := e.st.PipelineAttempts(ctx, run)
	out := map[string]bool{}
	for _, a := range list {
		out[a.ID] = true
	}
	return out, err
}

// visible lists the viewer's project entries, optionally of one run's attempts.
func (e *cacheEnv) visible(ctx context.Context, run string) ([]entries.Manifest, error) {
	var only map[string]bool
	if run != "" {
		var err error
		if only, err = e.runAttempts(ctx, run); err != nil {
			return nil, err
		}
	}
	all, err := e.st.CacheEntries(ctx, "")
	var out []entries.Manifest
	for _, m := range all {
		ns, derr := authz.DecodeNamespace(m.Namespace)
		if derr == nil && ns.Project == e.viewer.Project && (only == nil || only[m.Writer.AttemptID]) {
			out = append(out, m)
		}
	}
	return out, err
}

func cacheFlags(name string, args []string, def func(*flag.FlagSet)) (*flag.FlagSet, []string, error) {
	return parseArtifacts("--cache "+name, args, def)
}

func cacheList(args []string) int {
	var run *string
	fs, _, err := cacheFlags("list", args, func(fs *flag.FlagSet) { run = fs.String("run", "", "only entries written by this run") })
	if err != nil {
		return 1
	}
	env, err := openCache(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache list", false)
	}
	defer env.close()
	list, err := env.visible(context.Background(), *run)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache list", env.json)
	}
	views := make([]cacheEntryView, 0, len(list))
	for _, m := range list {
		views = append(views, viewOf(m))
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts --cache list", views)
	}
	for _, v := range views {
		fmt.Printf("%-8s %s  %12d B  %-12s last used %s\n", v.Spec, shortDigest(v.Key), v.Bytes, v.Trust, v.LastUsedAt)
	}
	return 0
}

// cacheUseView is one usage row as printed.
type cacheUseView struct {
	Attempt string `json:"attempt"`
	Job     string `json:"job"`
	Spec    string `json:"spec"`
	Key     string `json:"key,omitempty"`
	Result  string `json:"result"`
	Bytes   int64  `json:"bytes"`
	MS      int64  `json:"ms"`
}

func cacheShow(args []string) int {
	var run *string
	fs, pos, err := cacheFlags("show", args, func(fs *flag.FlagSet) { run = fs.String("run", "", "usage rows of this run's attempts") })
	if err != nil || (*run == "") == (len(pos) != 1) {
		fmt.Fprintln(os.Stderr, "usage: nself ci artifacts --cache show --run <run> | show <key> [--json]")
		return 1
	}
	env, err := openCache(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache show", false)
	}
	defer env.close()
	ctx := context.Background()
	if *run == "" {
		return cacheShowEntry(ctx, env, pos[0])
	}
	if _, err := env.runAttempts(ctx, *run); err != nil {
		return artifactsExit(err, "ci artifacts --cache show", env.json)
	}
	attempts, err := env.st.PipelineAttempts(ctx, *run)
	rows := []cacheUseView{}
	for _, a := range attempts {
		uses, uerr := env.es.Usage(ctx, a.ID)
		err = errors.Join(err, uerr)
		for _, u := range uses {
			rows = append(rows, cacheUseView{Attempt: a.ID, Job: a.JobID, Spec: u.Spec, Key: u.Key, Result: u.Result, Bytes: u.Bytes, MS: u.MS})
		}
	}
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache show", env.json)
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts --cache show", rows)
	}
	for _, r := range rows {
		fmt.Printf("%-8s %-8s %s  %d B  %d ms\n", r.Spec, r.Result, shortDigest(r.Key), r.Bytes, r.MS)
	}
	return 0
}

func cacheShowEntry(ctx context.Context, env *cacheEnv, prefix string) int {
	list, err := env.visible(ctx, "")
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache show", env.json)
	}
	for _, m := range list {
		if len(prefix) >= 8 && len(prefix) <= len(m.Key) && m.Key[:len(prefix)] == prefix {
			if env.json {
				return writeArtifactsJSON("ci artifacts --cache show", map[string]any{"entry": viewOf(m), "blobs": m.Blobs})
			}
			v := viewOf(m)
			fmt.Printf("key %s\nspec %s\ntrust %s\nprotection %s\nbytes %d\nattempt %s\nsecrets_received %t\nprivate_deps %t\nscanned_clean %t\ncreated %s\nlast used %s\n",
				v.Key, v.Spec, v.Trust, v.Protection, v.Bytes, v.Attempt, v.SecretsReceived, v.PrivateDeps, v.ScannedClean, v.CreatedAt, v.LastUsedAt)
			for _, b := range m.Blobs {
				fmt.Printf("blob %s %s %d\n", b.Path, shortDigest(b.SHA256), b.Size)
			}
			return 0
		}
	}
	return artifactsExit(&records.Error{ID: records.CodeNotFound, Reason: "not_found", Msg: "no such cache entry"}, "ci artifacts --cache show", env.json)
}
