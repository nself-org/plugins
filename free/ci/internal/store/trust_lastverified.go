package store

// Purpose: the read-only query behind the isolation last-verified record (P7-TRUST-25, R-DX-25): for each isolation
// level, the newest drill run in which that level's nested check passed. The answer comes from stored run evidence,
// so there is no write command and nothing here writes.
// Inputs:  the drill job id, the nested check ids to look for and the prefix of the line that lists the passed ids.
// Outputs: per check id, when it last passed, in which revision and on which runner platform.
// Constraints: run-level evidence only (attempt_id is NULL); only runs whose source_trust is owner or internal; newest first, read in pages of lastVerifiedPage rows, at most lastVerifiedPages pages; a body
// that does not decode, or a check of another job, is ignored; a check that is not listed as passed is never reported.

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// lastVerifiedPage is how many stored documents one query step reads, and lastVerifiedPages how many steps it takes
// (newest first), so an old pass stays in reach of a busy store yet the read stays bounded. A variable so tests can shrink it.
var (
	lastVerifiedPage  = 500
	lastVerifiedPages = 20
)

// Verification is one passing nested check found in stored drill evidence.
type Verification struct {
	CheckID    string
	FinishedAt time.Time
	Revision   string
	Platform   string
	// FailedAt is when the newest run that reported this check failed finished, set only when that run is newer than
	// the pass (D-1388); zero when the newest report of the check is the pass.
	FailedAt time.Time
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
// such run are absent from the map. A check that failed in a newer run than its newest pass carries that run's time
// in FailedAt.
func (s *Store) LastVerified(ctx context.Context, jobID, passedPrefix string, ids []string) (map[string]Verification, error) {
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string]Verification{}
	failed := map[string]time.Time{} // newest failure seen so far (newest first), per check without a pass yet
	var curTime, curRow int64
	for page := 0; page < lastVerifiedPages && len(out) < len(want); page++ {
		args := []any{jobID}
		cursor := ""
		if page > 0 {
			cursor = ` AND (created_at < ? OR (created_at = ? AND rowid < ?))`
			args = append(args, curTime, curTime, curRow)
		}
		args = append(args, lastVerifiedPage)
		rows, err := s.readers.QueryContext(ctx,
			`SELECT rowid, body, created_at FROM evidence WHERE attempt_id IS NULL AND instr(CAST(body AS TEXT), ?) > 0`+cursor+
				` ORDER BY created_at DESC, rowid DESC LIMIT ?`, args...)
		if err != nil {
			return nil, err
		}
		n := 0
		for rows.Next() {
			var body []byte
			if err = rows.Scan(&curRow, &body, &curTime); err != nil {
				_ = rows.Close()
				return nil, err
			}
			n++
			if len(out) < len(want) {
				readVerificationDoc(body, curTime, jobID, passedPrefix, want, out, failed)
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return nil, err
		}
		if n < lastVerifiedPage {
			break
		}
	}
	return out, nil
}

// readVerificationDoc folds one stored run document into out (first pass seen per id wins) and failed (first failure
// seen per id that has no pass yet).
func readVerificationDoc(body []byte, created int64, jobID, passedPrefix string, want map[string]bool, out map[string]Verification, failed map[string]time.Time) {
	var doc lastVerifiedDoc
	if json.Unmarshal(body, &doc) != nil || !trustedForVerification(doc.SourceTrust) {
		return
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
		for _, id := range failedIDs(c.Excerpt) {
			if _, have := out[id]; want[id] && !have {
				if _, seen := failed[id]; !seen {
					failed[id] = at
				}
			}
		}
		for _, id := range passedIDs(c.Excerpt, passedPrefix) {
			if _, have := out[id]; want[id] && !have {
				out[id] = Verification{CheckID: id, FinishedAt: at, Revision: doc.Revision, Platform: doc.Runner.Platform, FailedAt: failed[id]}
			}
		}
	}
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

// failedIDs reads the ids of the nested checks the drill excerpt lists as not passed: the indented "fail <id>" and
// "error <id>" lines (internal/drill writes them before the passed line).
func failedIDs(excerpt string) []string {
	var ids []string
	for _, line := range strings.Split(excerpt, "\n") {
		f := strings.Fields(strings.TrimRight(line, "\r"))
		if len(f) >= 2 && strings.HasPrefix(line, "  ") && (f[0] == "fail" || f[0] == "error") {
			ids = append(ids, strings.TrimSuffix(f[1], ":"))
		}
	}
	return ids
}
