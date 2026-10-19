package session

// Purpose: the secret values and output tail a lease carries: delivery of the
// declared values (fenced, once each, size-capped), the masks derived from them,
// and the masked tail the evidence excerpt is built from.
// Constraints: a value is masked before it is visible to the job.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func (s *Session) onSecrets(m protocol.Message, sec protocol.Secrets) {
	l, ok := s.active[sec.LeaseID]
	if !ok || m.Epoch == nil || *m.Epoch != l.lease.Epoch || m.LeaseID != sec.LeaseID {
		return
	}
	declared := map[string]bool{}
	for _, r := range l.lease.Secrets {
		declared[r.Ref] = true
	}
	// Decide what is taken, install the masks, and only then publish the values:
	// the job must never be able to print a value its masks do not yet cover.
	l.secMu.Lock()
	var refs, vals []string
	for _, v := range sec.Values {
		if l.secSeen[v.Ref] || !declared[v.Ref] || len(v.Value) > maxSecretValue || l.secBytes+len(v.Value) > maxSecretBytes {
			continue
		}
		l.secSeen[v.Ref] = true
		l.secBytes += len(v.Value)
		refs, vals = append(refs, v.Ref), append(vals, v.Value)
	}
	l.secMu.Unlock()
	for _, v := range vals {
		l.addMask(v)
	}
	l.secMu.Lock()
	for i, r := range refs {
		l.secVals[r] = vals[i]
	}
	l.secMu.Unlock()
	select {
	case l.secWake <- struct{}{}:
	default:
	}
}

const tailKeep = 4 << 10

func (l *activeLease) noteTail(text string) {
	l.tailMu.Lock()
	defer l.tailMu.Unlock()
	l.tail = append(l.tail, text...)
	l.tail = append(l.tail, '\n')
	if len(l.tail) > tailKeep {
		l.tail = l.tail[len(l.tail)-tailKeep:]
	}
}

func (l *activeLease) maskedTail() string {
	l.lw.Flush()
	l.tailMu.Lock()
	defer l.tailMu.Unlock()
	return strings.ToValidUTF8(string(l.tail), "")
}

func (l *activeLease) addMask(v string) {
	l.maskMu.Lock()
	defer l.maskMu.Unlock()
	for _, m := range secretMasks(v) {
		l.mask = append(l.mask, []byte(m))
	}
}

func (l *activeLease) redact(s string) string {
	return string(maskAll([]byte(s), l.masks()))
}

func (l *activeLease) masks() [][]byte {
	l.maskMu.Lock()
	defer l.maskMu.Unlock()
	return append([][]byte(nil), l.mask...)
}

// newJobRoot makes a private, uniquely named directory for one lease run: two
// coordinators may use the same lease id, and ids are not file-name safe.
func newJobRoot(state string, l protocol.Lease) (JobRoot, error) {
	base := filepath.Join(state, "jobs")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return JobRoot{}, err
	}
	dir, err := os.MkdirTemp(base, unsafeName.ReplaceAllString(l.LeaseID, "_")+"-"+strconv.FormatUint(l.Epoch, 10)+"-")
	return JobRoot{Dir: dir}, err
}

// shortNames counts how many delivered refs share each short name ("TOKEN" in
// "project/TOKEN"); a name two refs share is not aliased.
func shortNames(vals map[string]string) map[string]int {
	n := map[string]int{}
	for ref := range vals {
		if _, name, ok := strings.Cut(ref, "/"); ok {
			n[name]++
		}
	}
	return n
}

func shortValue(vals map[string]string, name string) string {
	for ref, v := range vals {
		if _, short, ok := strings.Cut(ref, "/"); ok && short == name {
			return v
		}
	}
	return ""
}

func missingSecrets(need []string, have map[string]string) bool {
	for _, n := range need {
		if _, ok := have[n]; !ok {
			return true
		}
	}
	return false
}
