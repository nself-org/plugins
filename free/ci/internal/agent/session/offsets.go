package session

// Purpose: hand the coordinator's `offset` answers to the job hook that is
// uploading a blob (P7-CACHE-04, Epic CACHE D7): the receiver answers a `digest`
// announce with the bytes it already holds, and may send another when a chunk
// arrives out of order. A sender needs them to resume.
// Inputs:  `offset` frames fenced to a running lease and its epoch.
// Outputs: a buffered channel per lease, read through OffsetSource.
// Constraints: the Run goroutine never blocks (a full channel drops the OLDEST
// answer, the newest one is the receiver's current position); an offset for an
// unknown lease or another epoch is ignored; session.Uploader is unchanged, so
// a hook asks for the channel with a type assertion.

import (
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// ErrHookFatal is wrapped by an After hook error that must end a passing job as an error: the hook
// owed the coordinator something the pass depends on (the declared artifacts), and could not deliver
// it. Any other After error is only logged.
var ErrHookFatal = errors.New("job hook failed and the job cannot pass")

// offsetBuffer is how many unread answers a lease keeps.
const offsetBuffer = 64

// OffsetSource is implemented by the Uploader a hook receives.
type OffsetSource interface {
	// Offsets carries the coordinator's offset answers for this lease.
	Offsets() <-chan protocol.Offset
}

func (s *Session) onOffset(m protocol.Message, o protocol.Offset) {
	l, ok := s.active[m.LeaseID]
	if !ok || m.Epoch == nil || *m.Epoch != l.lease.Epoch {
		return
	}
	for {
		select {
		case l.offsets <- o:
			return
		default:
		}
		select { // full: drop the oldest, then try again
		case <-l.offsets:
		default:
		}
	}
}

// Offsets returns the lease's offset answers.
func (u *uploader) Offsets() <-chan protocol.Offset { return u.al.offsets }

var _ OffsetSource = (*uploader)(nil)
