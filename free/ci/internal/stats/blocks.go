package stats

// Purpose: the cache, spend, storage, lease, node, outbox and store blocks of contract:ci.stats.
// Inputs:  the aggregated rows of the window (cache, spend) or of now (the rest).
// Outputs: the matching Doc fields.
// Constraints: a lease is stale when it expired under a leased or running attempt, self-fenced when its
//          fence epoch predates the current coordinator incarnation, and active otherwise; nodes, the
//          outbox and the store are coordinator-wide and appear only for a caller who may read it all.

import (
	"encoding/json"
	"os"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func (d *Doc) buildCacheSpend(rows store.StatsRows, o Options) {
	var c Cache
	seen := false
	for _, g := range rows.Cache {
		if !o.visible(g.Project) {
			continue
		}
		seen = true
		switch g.Key {
		case "hit":
			c.Hits += g.N
			c.BytesRestored += g.Sum
		case "miss":
			c.Misses += g.N
		case "saved":
			c.BytesSaved += g.Sum
		}
	}
	switch {
	case rows.Missing["cache_use"]:
		d.Reasons["cache"] = ReasonNotInstalled
	case !seen:
		d.Reasons["cache"] = ReasonNoRows
	default:
		if n := c.Hits + c.Misses; n > 0 {
			h := permille(c.Hits, n)
			c.HitPermille = &h
		} else {
			d.Reasons["cache.hit_permille"] = ReasonNoLookups
		}
		d.Cache = &c
	}
	spend := map[string]Spend{}
	for _, g := range rows.Ledger {
		if o.visible(g.Project) {
			s := spend[g.Key]
			spend[g.Key] = Spend{Attempts: s.Attempts + g.N, Micro: s.Micro + g.Sum}
		}
	}
	switch {
	case rows.Missing["ledger"]:
		d.Reasons["spend"] = ReasonNotInstalled
	case len(spend) == 0:
		d.Reasons["spend"] = ReasonNoRows
	default:
		for _, k := range []string{"known", "computed", "estimated", "unknown"} { // a real zero, the rows exist
			if _, ok := spend[k]; !ok {
				spend[k] = Spend{}
			}
		}
		d.Spend = spend
	}
}

func sumVisible(gs []store.StatGroup, o Options) (int64, bool) {
	var n int64
	var seen bool
	for _, g := range gs {
		if o.visible(g.Project) {
			n, seen = n+g.Sum, true
		}
	}
	return n, seen
}

func (d *Doc) buildPlatform(rows store.StatsRows, o Options) {
	st := &Storage{}
	for _, f := range []struct {
		key   string
		table string
		rows  []store.StatGroup
		dst   **int64
	}{{"storage.cas_bytes", "cache_entry", rows.CAS, &st.CASBytes}, {"storage.artifact_bytes", "cache_artifact", rows.Artifacts, &st.ArtifactBytes}} {
		switch n, seen := sumVisible(f.rows, o); {
		case rows.Missing[f.table]:
			d.Reasons[f.key] = ReasonNotInstalled
		case !seen:
			d.Reasons[f.key] = ReasonNoRows
		default:
			*f.dst = &n
		}
	}
	d.Storage = st

	now := o.Now.UnixNano()
	var l Leases
	for _, x := range rows.Leases {
		if !o.visible(x.Project) {
			continue
		}
		live := x.AttemptState == "leased" || x.AttemptState == "running"
		switch {
		case x.ExpiresAt < now && live:
			l.Stale++
		case live && x.Epoch > 0 && x.Epoch < rows.Incarnation<<32:
			l.SelfFenced++
		case x.ExpiresAt >= now:
			l.Active++
		}
	}
	d.Leases = &l

	if !o.Global || o.Project != "" {
		for _, k := range []string{"nodes", "outbox", "store"} {
			d.Reasons[k] = ReasonScoped
		}
		return
	}
	d.buildNodes(rows, o.Now)
	d.buildOutbox(rows, o.Now)
	d.buildStore(rows)
}

func (d *Doc) buildNodes(rows store.StatsRows, now time.Time) {
	switch {
	case rows.Missing["node"]:
		d.Reasons["nodes"] = ReasonNotInstalled
	case len(rows.Nodes) == 0:
		d.Reasons["nodes"] = ReasonNoRows
	default:
		n := &Nodes{ByState: map[string]int{}}
		for _, x := range rows.Nodes {
			n.ByState[x.State]++
			if x.State == "offline" && now.UnixNano()-x.UpdatedAt > int64(24*time.Hour) {
				n.OfflineOver24h++
			}
		}
		d.Nodes = n
	}
}

func (d *Doc) buildOutbox(rows store.StatsRows, now time.Time) {
	if rows.Missing["outbox"] {
		d.Reasons["outbox"] = ReasonNotInstalled
		return
	}
	ob := &Outbox{Pending: rows.OutboxPending}
	if rows.OutboxPending > 0 && rows.OutboxOldest > 0 {
		age := (now.UnixNano() - rows.OutboxOldest) / int64(time.Millisecond)
		ob.OldestAgeMs = &age
	} else {
		d.Reasons["outbox.oldest_age_ms"] = ReasonNonePending
	}
	d.Outbox = ob
}

func (d *Doc) buildStore(rows store.StatsRows) {
	var size int64
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if fi, err := os.Stat(rows.Path + suffix); err == nil {
			size += fi.Size()
		} else if suffix == "" {
			d.Reasons["store"] = ReasonStoreUnstated
			return
		}
	}
	s := &StoreInfo{SizeBytes: size}
	if free, ok := freeBytes(rows.Path); ok {
		s.FreeBytes = &free
	} else {
		d.Reasons["store.free_bytes"] = ReasonStoreUnstated
	}
	d.Store = s
}

// nodeSlots maps a node id and name to its advertised slot count; nodes that state none are absent.
func nodeSlots(nodes []store.StatNode) map[string]int {
	out := map[string]int{}
	for _, n := range nodes {
		var doc struct {
			Availability struct {
				Slots struct {
					Value *int `json:"value"`
				} `json:"slots"`
			} `json:"availability"`
		}
		if json.Unmarshal([]byte(n.Doc), &doc) == nil && doc.Availability.Slots.Value != nil {
			out[n.ID], out[n.Name] = *doc.Availability.Slots.Value, *doc.Availability.Slots.Value
		}
	}
	return out
}
