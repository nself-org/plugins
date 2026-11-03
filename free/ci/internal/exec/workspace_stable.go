package exec

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// acquireWorkspace creates an empty workspace and returns its lock release.
// Inputs: home, attempt ID and optional stable key. Outputs: directory, release, error.
// Constraints: busy stable paths fall back to work; stable paths remain outside work.
func acquireWorkspace(home, attemptID, key string) (dir string, release func(), err error) {
	release = func() {}
	dir = filepath.Join(home, "work", attemptID)
	if key != "" {
		stable := filepath.Join(home, "ws", fmt.Sprintf("%x", sha256.Sum256([]byte(key)))[:24])
		if err = os.MkdirAll(filepath.Dir(stable), 0700); err != nil {
			return "", release, err
		}
		var lock *os.File
		lock, err = os.OpenFile(stable+".lock", os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return "", release, err
		}
		var held bool
		held, err = lockWorkspace(lock)
		if err != nil || !held {
			_ = lock.Close()
			if err != nil {
				return "", release, err
			}
		} else {
			release = func() { unlockWorkspace(lock); _ = lock.Close() }
			dir = stable
			if err = os.RemoveAll(dir); err != nil {
				release()
				return "", func() {}, err
			}
		}
	}
	if err = os.MkdirAll(filepath.Dir(dir), 0700); err == nil {
		err = os.Mkdir(dir, 0700)
	}
	if err != nil {
		release()
		return "", func() {}, err
	}
	return dir, release, nil
}
