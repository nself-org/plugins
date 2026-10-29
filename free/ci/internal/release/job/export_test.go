package job

import "time"

// MapConflict exposes mapConflict to the external tests.
func MapConflict(err error) error { return mapConflict(err) }

// SetTiming shrinks the refresher's interval and the facts' maximum age.
func SetTiming(every, age time.Duration) (restore func()) {
	oe, oa := refreshEvery, maxAge
	refreshEvery, maxAge = every, age
	return func() { refreshEvery, maxAge = oe, oa }
}

// SetClock replaces the stash's clock.
func SetClock(f func() time.Time) (restore func()) {
	old := now
	now = f
	return func() { now = old }
}

// SetEntryKeyID sets the Signature.KeyID on a stored entry's facts, bypassing
// Evaluate. This simulates the race where facts were accepted by Guard but the
// signer is revoked before the admission step reads them.
func SetEntryKeyID(s *Stash, attemptID, keyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.m[attemptID]; ok {
		e.f.Evidence.Signature.KeyID = &keyID
	}
}
