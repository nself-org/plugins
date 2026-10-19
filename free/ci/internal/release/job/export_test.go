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
