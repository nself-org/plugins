package main

// Purpose: `nself ci artifacts --cache verify|prune` (P7-CACHE-02): the two
// views of artifacts_cache.go that scan or delete. verify re-hashes every blob
// and reports (it drops nothing); prune enforces the age and byte caps (admin
// role, because the byte cap is store wide), or lists what it would drop.
// Inputs:  argv after `--cache verify|prune`; the cache caps from the
// environment through entries.Store.Caps.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (corrupt entries found,
// usage, forbidden) or 2 (infrastructure).
// Constraints: see artifacts_cache.go.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
)

// cacheCheck is one entry's verify result.
type cacheCheck struct {
	Key    string `json:"key"`
	Spec   string `json:"spec"`
	State  string `json:"state"` // ok, missing, size_mismatch, corrupt
	Detail string `json:"detail,omitempty"`
}

func cacheVerify(args []string) int {
	var run *string
	fs, _, err := cacheFlags("verify", args, func(fs *flag.FlagSet) { run = fs.String("run", "", "only entries written by this run") })
	if err != nil {
		return 1
	}
	env, err := openCache(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache verify", false)
	}
	defer env.close()
	list, err := env.visible(context.Background(), *run)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache verify", env.json)
	}
	out, bad := []cacheCheck{}, 0
	for _, m := range list {
		c := checkEntry(env.blobs, m)
		if c.State != "ok" {
			bad++
		}
		out = append(out, c)
	}
	if env.json {
		code := writeArtifactsJSON("ci artifacts --cache verify", map[string]any{"checked": len(out), "corrupt": bad, "entries": out})
		if code == 0 && bad > 0 {
			return 1
		}
		return code
	}
	for _, c := range out {
		if c.State != "ok" {
			fmt.Printf("%-13s %s %s %s\n", c.State, c.Spec, shortDigest(c.Key), c.Detail)
		}
	}
	fmt.Printf("checked %d entr(ies), %d corrupt\n", len(out), bad)
	if bad > 0 {
		return 1
	}
	return 0
}

// checkEntry re-hashes every blob of m (cas.Open verifies size and digest).
func checkEntry(blobs *cas.Store, m entries.Manifest) cacheCheck {
	realm := cas.CacheRealm(string(m.Namespace))
	c := cacheCheck{Key: m.Key, Spec: m.Spec, State: "ok"}
	for _, b := range m.Blobs {
		info, err := blobs.Stat(realm, b.SHA256)
		switch {
		case errors.Is(err, os.ErrNotExist):
			c.State, c.Detail = "missing", b.Path
		case err != nil:
			c.State, c.Detail = "corrupt", b.Path
		case info.Size() != b.Size:
			c.State, c.Detail = "size_mismatch", b.Path
		default:
			rc, oerr := blobs.Open(realm, b.SHA256)
			if oerr == nil {
				_, oerr = io.Copy(io.Discard, rc)
				_ = rc.Close()
			}
			if oerr != nil {
				c.State, c.Detail = "corrupt", b.Path
			}
		}
		if c.State != "ok" {
			return c
		}
	}
	return c
}

func cachePrune(args []string) int {
	var dry *bool
	fs, _, err := cacheFlags("prune", args, func(fs *flag.FlagSet) { dry = fs.Bool("dry-run", false, "list what would be removed") })
	if err != nil {
		return 1
	}
	env, err := openCache(fs)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache prune", false)
	}
	defer env.close()
	if env.viewer.Role != authz.RoleAdmin {
		return artifactsExit(records.Failf(records.ReasonForbidden, "pruning the cache needs the admin role"), "ci artifacts --cache prune", env.json)
	}
	ctx := context.Background()
	if *dry {
		return cachePruneDry(ctx, env)
	}
	res, err := env.es.Prune(ctx, 30*time.Second)
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache prune", env.json)
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts --cache prune", map[string]any{"dry_run": false, "entries": res.Entries, "bytes": res.Bytes, "more": res.More})
	}
	more := ""
	if res.More {
		more = " (more remain; run again)"
	}
	fmt.Printf("removed %d entr(ies), %d B%s\n", res.Entries, res.Bytes, more)
	return 0
}

// cachePruneDry lists the entries the age cap and then the byte cap would drop
// (least recently used first), without touching anything.
func cachePruneDry(ctx context.Context, env *cacheEnv) int {
	all, err := env.st.CacheEntries(ctx, "")
	if err != nil {
		return artifactsExit(err, "ci artifacts --cache prune", env.json)
	}
	maxBytes, maxAge := env.es.Caps()
	sort.SliceStable(all, func(i, j int) bool { return all[i].LastUsedAt < all[j].LastUsedAt })
	cutoff, total := time.Now().Add(-maxAge).UnixNano(), int64(0)
	for _, m := range all {
		total += m.Bytes()
	}
	drop, bytes := []cacheEntryView{}, int64(0)
	for _, m := range all {
		if m.LastUsedAt < cutoff || total-bytes > maxBytes {
			drop, bytes = append(drop, viewOf(m)), bytes+m.Bytes()
		}
	}
	if env.json {
		return writeArtifactsJSON("ci artifacts --cache prune", map[string]any{"dry_run": true, "entries": drop, "bytes": bytes})
	}
	for _, v := range drop {
		fmt.Printf("would remove %s %s (%d B)\n", v.Spec, shortDigest(v.Key), v.Bytes)
	}
	fmt.Printf("would remove %d entr(ies), %d B\n", len(drop), bytes)
	return 0
}
