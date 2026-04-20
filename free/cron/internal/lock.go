package internal

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// JobLock is the interface implemented by both the Postgres advisory lock
// and the filesystem fallback lock.
type JobLock interface {
	// TryLock attempts to acquire a lock for jobName.
	// Returns (true, releaseFunc, nil) when acquired.
	// Returns (false, nil, nil) when already held (not an error).
	// Returns (false, nil, err) on infrastructure failure.
	TryLock(ctx context.Context, jobName string) (acquired bool, release func(), err error)
}

// AdvisoryLock implements JobLock using Postgres pg_try_advisory_lock.
// pg_try_advisory_lock is non-blocking (returns immediately if not acquirable).
// The lock key is derived from hashtext(jobName) — no user input injection.
type AdvisoryLock struct {
	Pool *pgxpool.Pool
}

// hashJob derives a stable int64 lock key from a job name.
// Uses md5(jobName)[0:8] as a deterministic 64-bit integer.
// This mirrors Postgres hashtext() — safe against user-controlled job names
// because the hash is one-way and does not influence SQL structure.
func hashJob(jobName string) int64 {
	h := md5.Sum([]byte(jobName))
	return int64(binary.BigEndian.Uint64(h[:8]))
}

// TryLock acquires a session-level Postgres advisory lock for jobName.
// The lock is automatically released when the connection is returned to the pool,
// or explicitly via the release function.
// Returns an error if the pool is nil (caller should fall back to FileLock).
func (l *AdvisoryLock) TryLock(ctx context.Context, jobName string) (bool, func(), error) {
	if l.Pool == nil {
		return false, nil, fmt.Errorf("advisory lock pool is nil")
	}

	lockKey := hashJob(jobName)

	conn, err := l.Pool.Acquire(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("acquiring connection for advisory lock: %w", err)
	}

	var acquired bool
	err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	if err != nil {
		conn.Release()
		return false, nil, fmt.Errorf("pg_try_advisory_lock(%d): %w", lockKey, err)
	}

	if !acquired {
		conn.Release()
		return false, nil, nil
	}

	release := func() {
		// Unlock before releasing the connection back to the pool.
		_, unlockErr := conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
		if unlockErr != nil {
			log.Printf("advisory unlock warning for job %q: %v", jobName, unlockErr)
		}
		conn.Release()
	}

	return true, release, nil
}

// --- Filesystem fallback lock ---

// FileLock implements JobLock using a per-job in-memory mutex + optional lock file.
// Used when the Postgres pool is unreachable.
type FileLock struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
	dir   string // directory for .lock files (empty = /tmp/nself-cron)
}

// NewFileLock creates a FileLock. dir is the directory for lock files;
// pass "" to use /tmp/nself-cron.
func NewFileLock(dir string) *FileLock {
	if dir == "" {
		dir = "/tmp/nself-cron"
	}
	return &FileLock{
		locks: make(map[string]*sync.Mutex),
		dir:   dir,
	}
}

func (f *FileLock) getMu(jobName string) *sync.Mutex {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks[jobName] == nil {
		f.locks[jobName] = &sync.Mutex{}
	}
	return f.locks[jobName]
}

// TryLock acquires an in-memory non-blocking lock plus creates a lock file.
func (f *FileLock) TryLock(_ context.Context, jobName string) (bool, func(), error) {
	mu := f.getMu(jobName)
	if !mu.TryLock() {
		return false, nil, nil
	}

	// Create lock file (best-effort — in-memory mutex is authoritative).
	lockPath := filepath.Join(f.dir, sanitizeName(jobName)+".lock")
	if err := os.MkdirAll(f.dir, 0o750); err == nil {
		_ = os.WriteFile(lockPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o640)
	}

	release := func() {
		_ = os.Remove(lockPath)
		mu.Unlock()
	}
	return true, release, nil
}

// sanitizeName converts a job name to a filesystem-safe string.
func sanitizeName(s string) string {
	out := make([]byte, len(s))
	for i, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			out[i] = c
		} else {
			out[i] = '_'
		}
	}
	return string(out)
}

// --- OverlapCounter tracks cron_job_overlap_skipped metric ---

// OverlapCounter records per-job overlap skip counts.
// Integrates with the Prometheus counter registered in runner.go.
type OverlapCounter struct {
	mu     sync.Mutex
	counts map[string]int64
	// hook for tests / metric injection
	IncrementHook func(jobName string)
}

// NewOverlapCounter creates a new counter.
func NewOverlapCounter() *OverlapCounter {
	return &OverlapCounter{
		counts: make(map[string]int64),
	}
}

// Increment records one overlap skip for jobName and calls IncrementHook if set.
func (c *OverlapCounter) Increment(jobName string) {
	c.mu.Lock()
	c.counts[jobName]++
	c.mu.Unlock()
	if c.IncrementHook != nil {
		c.IncrementHook(jobName)
	}
}

// Count returns the total skip count for jobName.
func (c *OverlapCounter) Count(jobName string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counts[jobName]
}

// --- LockManager wraps advisory lock with filesystem fallback ---

// LockManager selects AdvisoryLock when DB is available, FileLock otherwise.
// Alert rule: overlap > 3 times in 1h triggers a page.
type LockManager struct {
	advisory *AdvisoryLock
	file     *FileLock
	counter  *OverlapCounter
}

// NewLockManager creates a LockManager.
func NewLockManager(pool *pgxpool.Pool, counter *OverlapCounter) *LockManager {
	return &LockManager{
		advisory: &AdvisoryLock{Pool: pool},
		file:     NewFileLock(""),
		counter:  counter,
	}
}

// TryLockWithFallback tries the advisory lock; on DB failure falls back to
// the filesystem lock and logs a WARN.
func (m *LockManager) TryLockWithFallback(ctx context.Context, jobName string) (bool, func(), error) {
	acquired, release, err := m.advisory.TryLock(ctx, jobName)
	if err != nil {
		log.Printf("WARN: advisory lock unavailable for job %q (%v); falling back to filesystem lock", jobName, err)
		return m.file.TryLock(ctx, jobName)
	}
	return acquired, release, nil
}

// RecordOverlap increments the overlap counter and emits a log warning.
func (m *LockManager) RecordOverlap(jobName string) {
	m.counter.Increment(jobName)
	log.Printf("WARN: cron job %q skipped — previous instance still running (overlap count: %d)",
		jobName, m.counter.Count(jobName))
}

// --- Alertmanager rule ---
// The alert rule for cron_job_overlap_skipped > 3 in 1h is defined in:
//   plugins/free/cron/alerts/cron-overlap.yaml
// (See T05 acceptance criteria — promtool check rules must pass)

// OverlapAlertThreshold is the per-job overlap threshold before alerting.
const OverlapAlertThreshold = 3

// OverlapAlertWindow is the rolling window for overlap alerting.
const OverlapAlertWindow = time.Hour

// JobRunContext is passed to each job execution to carry locking infrastructure.
type JobRunContext struct {
	Lock    *LockManager
	JobName string
}

// SkipIfOverlapping acquires the job lock and returns a release function.
// If the lock cannot be acquired (overlap detected), it records the skip
// and returns (false, nil).
func SkipIfOverlapping(ctx context.Context, jobCtx JobRunContext) (shouldRun bool, release func()) {
	acquired, releaseFn, err := jobCtx.Lock.TryLockWithFallback(ctx, jobCtx.JobName)
	if err != nil {
		log.Printf("ERROR: lock failure for job %q: %v — running job anyway (fail open)", jobCtx.JobName, err)
		return true, func() {}
	}
	if !acquired {
		jobCtx.Lock.RecordOverlap(jobCtx.JobName)
		return false, nil
	}
	return true, releaseFn
}
