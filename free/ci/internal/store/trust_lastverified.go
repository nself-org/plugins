package store

// Purpose: the read-only query behind the isolation last-verified record (P7-TRUST-25, R-DX-25): for each isolation
// level, the newest drill run in which that level's nested check passed. The answer comes from stored run evidence,
// so there is no write command and nothing here writes.
// Inputs:  the drill job id, the nested check ids to look for and the prefix of the line that lists the passed ids.
// Outputs: per check id, when it last passed, in which revision and on which runner platform.
// Constraints: run-level evidence only (attempt_id is NULL); only runs whose source_trust is owner or internal; newest first, at most lastVerifiedScan rows read; a body
// that does not decode, or a check of another job, is ignored; a check that is not listed as passed is never reported.

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// lastVerifiedScan bounds how many stored documents one query reads.
const lastVerifiedScan = 500

// Verification is one passing nested check found in stored drill evidence.
type Verification struct {
	CheckID    string
	FinishedAt time.Time
	Revision   string
	Platform   string
}

// lastVerifiedDoc is the part of a ci.evidence/v1 document this query reads.
type lastVerifiedDoc struct {
	Revision    string `json:"revision"`
	SourceTrust string `json:"source_trust"`
	Runner      struct {
		Platform string `json:"platform"`
	} `json:"runner"`
	Timings struct {
		FinishedAt *string `json:"finished_at"`
	} `json:"timings"`
	Checks []struct {
		JobID   string `json:"job_id"`
		Kind    string `json:"kind"`
		Excerpt string `json:"excerpt"`
	} `json:"checks"`
}

// LastVerified returns, per wanted check id, the newest run whose drill job (job id jobID, kind drill) listed that id
// on its passed line (the line that starts with passedPrefix; the last such line of an excerpt wins). Ids with no
// such run are absent from the map.
func (s *Store) LastVerified(ctx context.Context, jobID, passedPrefix string, ids []string) (map[string]Verification, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string]Verification{}
	rows, err := s.readers.QueryContext(ctx,
		`SELECT body, created_at FROM evidence WHERE attempt_id IS NULL AND instr(CAST(body AS TEXT), ?) > 0 ORDER BY created_at DESC LIMIT ?`,
		jobID, lastVerifiedScan)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() && len(out) < len(want) {
		var body []byte
		var created int64
		if err = rows.Scan(&body, &created); err != nil {
			return nil, err
		}
		var doc lastVerifiedDoc
		if json.Unmarshal(body, &doc) != nil || !trustedForVerification(doc.SourceTrust) {
			continue
		}
		at := time.Unix(0, created).UTC()
		if doc.Timings.FinishedAt != nil {
			if t, perr := time.Parse(time.RFC3339, *doc.Timings.FinishedAt); perr == nil {
				at = t.UTC()
			}
		}
		for _, c := range doc.Checks {
			if c.JobID != jobID || c.Kind != "drill" {
				continue
			}
			for _, id := range passedIDs(c.Excerpt, passedPrefix) {
				if _, have := out[id]; want[id] && !have {
					out[id] = Verification{CheckID: id, FinishedAt: at, Revision: doc.Revision, Platform: doc.Runner.Platform}
				}
			}
		}
	}
	return out, rows.Err()
}

// trustedForVerification is true for runs of the owner or the organisation. A run of a collaborator or an untrusted
// source can name its job like the drill, so its evidence never proves an isolation level.
func trustedForVerification(trust string) bool { return trust == "owner" || trust == "internal" }

// passedIDs reads the ids on the last line of an excerpt that starts with prefix.
func passedIDs(excerpt, prefix string) []string {
	var ids []string
	for _, line := range strings.Split(excerpt, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), prefix); ok {
			ids = strings.Fields(rest)
		}
	}
	return ids
}
