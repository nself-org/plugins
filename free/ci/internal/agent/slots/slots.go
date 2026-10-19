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
	"crypto/rand"
	"encoding/hex"
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
	// Token tells two acquisitions of the same index by the same owner apart, so
	// a stale Release cannot free a newer slot.
	Token string `json:"token"`
	Owner
}

// tableFile is slots.json. Cap is the host's slot count, fixed by whoever first
// opened the table (or by SetCap, the operator's lever: `nself-ci-agent slots
// --set N`): later Opens can lower their own count but never raise it, so a
// coordinator that picks the agent's command line cannot widen the cap.
type tableFile struct {
	Cap   int     `json:"cap"`
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
	token string
}

// Open returns the table of stateDir with total slots (at least 1).
func Open(stateDir string, total int) (*Table, error) {
	if total < 1 {
		return nil, fmt.Errorf("slot count %d: want at least 1", total)
	}
	if err := nodekey.EnsureDir(stateDir); err != nil {
		return nil, err
	}
	t := &Table{dir: stateDir, total: total}
	err := t.locked(func(tf *tableFile) (bool, error) {
		switch {
		case tf.Cap <= 0:
			tf.Cap = total
			return true, nil
		case tf.Cap < t.total:
			t.total = tf.Cap
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// SetCap sets the host's slot count in stateDir, raising or lowering it (the
// operator's lever; Open only ever lowers). Slots held beyond a lowered cap
// finish normally and are not reissued.
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

// Status reports the host's recorded cap (0 when none was set yet) and the
// slots held by live owners.
func Status(stateDir string) (capacity, used int, err error) {
	if err := nodekey.EnsureDir(stateDir); err != nil {
		return 0, 0, err
	}
	t := &Table{dir: stateDir, total: 1}
	err = t.locked(func(tf *tableFile) (bool, error) { capacity, used = tf.Cap, len(tf.Slots); return false, nil })
	return capacity, used, err
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
		limit := t.total
		if tf.Cap > 0 && tf.Cap < limit {
			limit = tf.Cap // the operator lowered the cap after this handle opened
		}
		if len(tf.Slots) >= limit {
			return false, &Error{Reason: "slots_full"}
		}
		for i := 0; i < limit+len(tf.Slots); i++ {
			if !used[i] {
				tok := newToken()
				tf.Slots = append(tf.Slots, entry{Index: i, Since: time.Now().UTC(), Token: tok, Owner: owner})
				got = &Slot{Index: i, owner: owner, token: tok}
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
			if e.Index == s.Index && e.Owner == s.owner && e.Token == s.token {
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

func newToken() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
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
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync() // a power loss must not leave an empty table under the final name
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(t.dir, tableName)); err != nil {
		return err
	}
	if d, err := os.Open(t.dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
