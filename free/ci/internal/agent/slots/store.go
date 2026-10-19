package slots

// Purpose: the table file and its lock: reading and atomically rewriting
// slots.json under the flock, reclaiming dead owners, and the operator's reads
// and writes of the host cap.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
)

func SetCap(stateDir string, total int) error {
	if total < 1 {
		return fmt.Errorf("slot count %d: want at least 1", total)
	}
	if err := nodekey.EnsureDir(stateDir); err != nil {
		return err
	}
	t := &Table{dir: stateDir, total: total}
	return t.locked(func(tf *tableFile) (bool, error) { tf.Cap = total; return true, nil })
}

func Status(stateDir string) (capacity, used int, err error) {
	if err := nodekey.EnsureDir(stateDir); err != nil {
		return 0, 0, err
	}
	t := &Table{dir: stateDir, total: 1}
	err = t.locked(func(tf *tableFile) (bool, error) { capacity, used = tf.Cap, len(tf.Slots); return false, nil })
	return capacity, used, err
}

func newToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func WithLock(dir, name string, fn func() error) error {
	lock, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := lockFile(lock); err != nil {
		return err
	}
	defer unlockFile(lock)
	return fn()
}

func dropDead(tf *tableFile) bool {
	live := tf.Slots[:0]
	for _, e := range tf.Slots {
		if holderLive(e.Owner) {
			live = append(live, e)
		}
	}
	dirty := len(live) != len(tf.Slots)
	tf.Slots = live
	return dirty
}

func holderLive(o Owner) bool {
	if !alive(o.PID) {
		return false
	}
	now := startTime(o.PID)
	return o.Start == "" || now == "" || now == o.Start
}
