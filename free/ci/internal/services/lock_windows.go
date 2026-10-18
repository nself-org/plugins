//go:build windows

package services

// Purpose: Windows has no flock; service dependencies are not offered there
// (the funnel refuses with E637), so the lock only reports that.

type ownerLock struct{}

func acquireLock(string) (*ownerLock, bool, error) {
	return nil, false, fail(CodeNoDocker, "service dependencies need a POSIX host with Docker")
}

func (l *ownerLock) Unlock() {}
