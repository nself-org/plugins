package run

// Purpose: a hosted runner never receives a dirty-tree snapshot (P7-AUTO-13).
// A hosted runner fetches the revision from the forge, where a snapshot never
// exists; when placement picks one for a run whose nodes check out a snapshot,
// the lease carries the commit the run started from instead, the run expects
// evidence naming that commit, and the run output says the dirty changes were
// not included.
// Inputs:  the attempt id and the revision the dispatcher put on the lease.
// Outputs: the revision the hosted lease checks out; a run note.
// Constraints: a lease to an enrolled node is never changed; a run without a
// snapshot is never changed. The note is printed once per run.

import "slices"

// HostedDirtyNote is the run note when a hosted runner gets HEAD without the
// uncommitted changes of a dirty tree.
const HostedDirtyNote = "dirty changes not included: hosted runner"

// hostedRevision is the transport adapter's hosted hook for this run.
func (rd *runDispatch) hostedRevision(attemptID, revision string) string {
	snap := rd.sess.Checkout
	if snap == "" || snap == rd.sess.Revision || revision != snap {
		return revision
	}
	if ra := rd.sink.attempt(attemptID); ra != nil {
		ra.revision = rd.sess.Revision // the hosted runner's evidence names the commit it fetched
	}
	rd.note(HostedDirtyNote)
	return rd.sess.Revision
}

func (rd *runDispatch) note(n string) {
	rd.notesMu.Lock()
	defer rd.notesMu.Unlock()
	if !slices.Contains(rd.notes, n) {
		rd.notes = append(rd.notes, n)
	}
}

// dispatchNotes returns the delivery notes of a run whose dispatcher is still open.
func dispatchNotes(runID string) []string {
	v, ok := runDispatchers.Load(runID)
	if !ok {
		return nil
	}
	rd := v.(*runDispatch)
	rd.notesMu.Lock()
	defer rd.notesMu.Unlock()
	return append([]string(nil), rd.notes...)
}
