package session

import "time"

// Clock is the session's time source; tests inject a manual one.
type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) Ticker
}

// Ticker is a stoppable periodic channel.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) NewTicker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time { return r.t.C }
func (r realTicker) Stop()               { r.t.Stop() }
