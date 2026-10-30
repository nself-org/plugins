package fleet

// Purpose: merge one node's registry entry, its reply and the local results into a Row.
// Inputs:  registry.Node, Reply, the recent local results and the time of the pass.
// Outputs: Row.
// Constraints: pure. Readiness prefers the node's readiness block (contract:ci.readiness v1) and
//          falls back to the ready fields of its demand document. Only watch hosts publish
//          demand.json: a node that answered without one runs no watcher (watcher: none).

import (
	"encoding/json"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/demand"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/readiness"
)

// staleAfter is how many watcher intervals may pass before a demand document reads as stale.
const staleAfter = 3

// defaultInterval ages a document whose interval is unknown.
const defaultInterval = 30 * time.Second

// BuildRow merges the sources of one node.
func BuildRow(n registry.Node, r Reply, results []Result, now time.Time) Row {
	row := Row{
		Node: n.Record.ID, Name: n.Record.Name, State: n.Record.State,
		AgentVersion: n.Capability.Protocol.AgentVersion,
		Pools:        pools.FromAssigned(n.Capability.Identity.Labels),
		ReadyWhy:     []string{}, LastResults: append([]Result{}, results...),
	}
	if row.Pools == nil {
		row.Pools = []string{}
	}
	if len(row.LastResults) > MaxResults {
		row.LastResults = row.LastResults[:MaxResults]
	}
	if r.Err != nil {
		row.Source, row.Unreachable, row.Watcher = SourceUnreachable, reason(r.Err), WatcherUnknown
		row.ReadyWhy = []string{"unreachable: " + row.Unreachable}
		return row
	}
	row.Source, row.Watcher = SourceLive, WatcherNone
	if doc, ok := parseDemand(r.Docs.Demand); ok {
		row.Demand = &doc
		row.Watcher = watcherState(doc, now)
		row.LastPass = doc.Watcher.LastPass
		row.Queued, row.Running = intPtr(doc.Queued), intPtr(doc.Running)
		row.Ready = boolPtr(doc.Ready)
		row.ReadyWhy = append(row.ReadyWhy, doc.ReadyWhy...)
	} else if len(r.Docs.Demand) > 0 {
		row.ReadyWhy = []string{"demand document unreadable"}
	}
	if rep, ok := parseReadiness(r.Docs.Readiness); ok {
		row.Ready = boolPtr(rep.Ready)
		row.ReadyWhy = append([]string{}, rep.Why...)
	}
	if row.Ready == nil && len(row.ReadyWhy) == 0 {
		row.ReadyWhy = []string{"no readiness published (readiness is per watch host)"}
	}
	return row
}

// parseDemand decodes a demand document; ok is false when there is none or it is torn.
func parseDemand(b []byte) (demand.Document, bool) {
	var doc demand.Document
	if len(b) == 0 || json.Unmarshal(b, &doc) != nil {
		return demand.Document{}, false
	}
	if _, err := time.Parse(time.RFC3339, doc.TS); err != nil {
		return demand.Document{}, false
	}
	if doc.ReadyWhy == nil {
		doc.ReadyWhy = []string{}
	}
	return doc, true
}

// parseReadiness decodes a readiness block of the current schema.
func parseReadiness(b []byte) (readiness.Report, bool) {
	var rep readiness.Report
	if len(b) == 0 || json.Unmarshal(b, &rep) != nil || rep.Schema != readiness.SchemaID {
		return readiness.Report{}, false
	}
	if rep.Why == nil {
		rep.Why = []string{}
	}
	return rep, true
}

// watcherState is alive while the document is younger than staleAfter intervals.
func watcherState(doc demand.Document, now time.Time) string {
	ts, err := time.Parse(time.RFC3339, doc.TS)
	if err != nil || !doc.Watcher.Alive {
		return WatcherStale
	}
	interval := time.Duration(doc.Watcher.IntervalSec * float64(time.Second))
	if interval <= 0 {
		interval = defaultInterval
	}
	if now.Sub(ts) > staleAfter*interval {
		return WatcherStale
	}
	return WatcherAlive
}

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }
