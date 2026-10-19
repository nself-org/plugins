// Package slots is the host-scoped slot table (Epic D15, X10, R-AR-21).
//
// Purpose: cap the leases a node runs at once across every coordinator and
// every carrier, by holding one table per agent state directory.
// Inputs:  the state dir, the slot count, and an Owner per Acquire.
// Outputs: a Slot (its index feeds the job UID), E654 when the table is full.
// Constraints: the table is a JSON file rewritten atomically under an
// exclusive flock on <state>/slots.lock (flock_unix.go is the only caller of
// syscall.Flock; windows gets a stub); a holder is dead, and its slots are
// reclaimed on the next Acquire, when its pid is gone or its start time no
// longer matches. A holder that is alive keeps its slot however long it runs.
package slots

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
)

const (
	lockName  = "slots.lock"
	tableName = "slots.json"
)

// Owner names who holds a slot.
type Owner struct {
	PID         int    `json:"pid"`
	Start       string `json:"start"`
	Coordinator string `json:"coordinator"`
	Lease       string `json:"lease"`
}

type entry struct {
	Index int       `json:"index"`
	Since time.Time `json:"since"`
	Owner
}

type tableFile struct {
	Slots []entry `json:"slots"`
}

// Table is one agent's slot table.
type Table struct {
	dir   string
	total int
}

// Slot is an acquired slot; pass it to Release.
type Slot struct {
	Index int
	owner Owner
}

// Open returns the table of stateDir with total slots (at least 1).
func Open(stateDir string, total int) (*Table, error) {
	if total < 1 {
		return nil, fmt.Errorf("slot count %d: want at least 1", total)
	}
	if err := nodekey.EnsureDir(stateDir); err != nil {
		return nil, err
	}
	return &Table{dir: stateDir, total: total}, nil
}

// Total is the slot count.
func (t *Table) Total() int { return t.total }

// Self is the Owner of the calling process for one lease.
func Self(coordinator, lease string) Owner {
	pid := os.Getpid()
	return Owner{PID: pid, Start: startTime(pid), Coordinator: coordinator, Lease: lease}
}

// Acquire takes the lowest free slot for owner, or returns *Error E654.
func (t *Table) Acquire(owner Owner) (*Slot, error) {
	var got *Slot
	err := t.locked(func(tf *tableFile) (bool, error) {
		used := map[int]bool{}
		for _, e := range tf.Slots {
			used[e.Index] = true
		}
		for i := 0; i < t.total; i++ {
			if !used[i] {
				tf.Slots = append(tf.Slots, entry{Index: i, Since: time.Now().UTC(), Owner: owner})
				got = &Slot{Index: i, owner: owner}
				return true, nil
			}
		}
		return false, &Error{Reason: "slots_full"}
	})
	return got, err
}

// Release frees the slot. Releasing twice, or a slot another owner now holds,
// changes nothing.
func (t *Table) Release(s *Slot) error {
	if s == nil {
		return nil
	}
	return t.locked(func(tf *tableFile) (bool, error) {
		for i, e := range tf.Slots {
			if e.Index == s.Index && e.Owner == s.owner {
				tf.Slots = append(tf.Slots[:i], tf.Slots[i+1:]...)
				return true, nil
			}
		}
		return false, nil
	})
}

// Used is the number of slots held by live owners.
func (t *Table) Used() (int, error) {
	n := 0
	err := t.locked(func(tf *tableFile) (bool, error) { n = len(tf.Slots); return false, nil })
	return n, err
}

// WithLock runs fn holding an exclusive lock on <dir>/<name>, for state files
// other than the table (coordinators.json) that several agent processes
// rewrite. It is here because the OS lock call lives in this package only.
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

// locked runs fn on the live table under the file lock; fn reports whether the
// table must be written back. Dead owners are dropped (and persisted) first.
func (t *Table) locked(fn func(*tableFile) (bool, error)) error {
	lock, err := os.OpenFile(filepath.Join(t.dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := lockFile(lock); err != nil {
		var se *Error
		if errors.As(err, &se) {
			return err
		}
		return fmt.Errorf("lock %s: %w", lock.Name(), err)
	}
	defer unlockFile(lock)
	tf, err := t.read()
	if err != nil {
		return err
	}
	dirty := dropDead(tf)
	write, err := fn(tf)
	if err != nil {
		if dirty {
			_ = t.write(tf) // reclaiming dead owners is worth keeping even when the request fails
		}
		return err
	}
	if write || dirty {
		return t.write(tf)
	}
	return nil
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

// holderLive is false when the pid is gone or now belongs to a younger process.
func holderLive(o Owner) bool {
	if !alive(o.PID) {
		return false
	}
	now := startTime(o.PID)
	return o.Start == "" || now == "" || now == o.Start
}

func (t *Table) read() (*tableFile, error) {
	raw, err := os.ReadFile(filepath.Join(t.dir, tableName))
	if errors.Is(err, fs.ErrNotExist) {
		return &tableFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var tf tableFile
	if err := json.Unmarshal(raw, &tf); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", tableName, err)
	}
	return &tf, nil
}

func (t *Table) write(tf *tableFile) error {
	sort.Slice(tf.Slots, func(i, j int) bool { return tf.Slots[i].Index < tf.Slots[j].Index })
	raw, err := json.Marshal(tf)
	if err != nil {
		return err
	}
	tmp := filepath.Join(t.dir, tableName+"."+strconv.Itoa(os.Getpid())+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(t.dir, tableName))
}
