package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Options configures the local-disk store. ReaderVersion supports version-skew tests.
type Options struct {
	ReaderVersion int
	TTL           time.Duration
	Clock         func() time.Time
}

// Store owns one writer connection and a separate read pool. The database requires local disk.
type Store struct {
	writer, readers *sql.DB
	path            string
	ttl             time.Duration
	now             func() time.Time
	mu              sync.Mutex
	lastBeat        map[string]time.Time
	sealer          AuditSealer
}

func defaultPath() (string, error) {
	home := os.Getenv("NSELF_CI_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return filepath.Join(home, "state.db"), nil
}

func openDB(path string, max int) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if max == 1 {
		dsn += "&_txlock=immediate"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(max)
	db.SetMaxIdleConns(max)
	if err = db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Open creates or upgrades a local SQLite database, using WAL and restrictive modes.
func Open(path string, opts Options) (*Store, error) {
	var err error
	if path == "" {
		path, err = defaultPath()
		if err != nil {
			return nil, err
		}
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if err = os.Chmod(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	writer, err := openDB(path, 1)
	if err != nil {
		return nil, err
	}
	if _, err = writer.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = writer.Close()
		return nil, err
	}
	version := opts.ReaderVersion
	if version == 0 {
		version = 1
	}
	if err = migrate(context.Background(), writer, version); err != nil {
		_ = writer.Close()
		return nil, err
	}
	readers, err := openDB(path, 4)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := opts.Clock
	if now == nil {
		now = time.Now
	}
	return &Store{writer: writer, readers: readers, path: path, ttl: ttl, now: now, lastBeat: map[string]time.Time{}}, nil
}

func (s *Store) Close() error {
	a := s.readers.Close()
	b := s.writer.Close()
	return errors.Join(a, b)
}

func (s *Store) write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// AuditRow stores references and decision metadata only, never secret material.
type AuditRow struct {
	Seq                           int64
	TS                            int64
	Actor, Action, Target, Detail string
}
type AuditChain struct{ Head, Signature []byte }
type AuditSealer func(prevHead []byte, row AuditRow) (AuditChain, error)

func (s *Store) SetAuditSealer(sealer AuditSealer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealer = sealer
}

func (s *Store) appendAudit(ctx context.Context, tx *sql.Tx, actor, action, target, detail string) error {
	if actor == "" || action == "" || target == "" || strings.Contains(strings.ToLower(detail), "secret=") {
		return coded("E607", "invalid audit metadata")
	}
	var prev []byte
	err := tx.QueryRowContext(ctx, "SELECT head FROM audit ORDER BY seq DESC LIMIT 1").Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	row := AuditRow{TS: s.now().UnixNano(), Actor: actor, Action: action, Target: target, Detail: detail}
	res, err := tx.ExecContext(ctx, "INSERT INTO audit(ts,actor,action,target,detail,prev_head) VALUES (?,?,?,?,?,?)", row.TS, actor, action, target, detail, prev)
	if err != nil {
		return err
	}
	row.Seq, err = res.LastInsertId()
	if err != nil {
		return err
	}
	s.mu.Lock()
	sealer := s.sealer
	s.mu.Unlock()
	if sealer != nil {
		chain, err := sealer(prev, row)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE audit SET head=?,signature=? WHERE seq=?", chain.Head, chain.Signature, row.Seq)
		return err
	}
	return nil
}

// AppendAudit records an operator or coordinator decision atomically.
func (s *Store) AppendAudit(ctx context.Context, actor, action, target, detail string) error {
	return s.write(ctx, func(tx *sql.Tx) error { return s.appendAudit(ctx, tx, actor, action, target, detail) })
}
