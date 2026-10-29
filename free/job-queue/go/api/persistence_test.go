package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type snapshotDB struct {
	queries []string
	args    [][]any
	err     error
}

func (d *snapshotDB) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	d.queries = append(d.queries, q)
	d.args = append(d.args, args)
	return pgconn.NewCommandTag("INSERT 0 1"), d.err
}
func (*snapshotDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query")
}
func (*snapshotDB) QueryRow(context.Context, string, ...any) pgx.Row { panic("unexpected QueryRow") }

type redisHook struct {
	commands []string
	args     [][]any
	err      error
}

func (*redisHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *redisHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		h.commands = append(h.commands, cmd.Name())
		h.args = append(h.args, cmd.Args())
		return h.err
	}
}
func (*redisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestEnqueuePersistsSnapshotBeforeExecution(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "database failure"}[fail], func(t *testing.T) {
			s := newTestServer()
			d := &snapshotDB{}
			if fail {
				d.err = errors.New("offline")
			}
			s.db = d
			hook := &redisHook{}
			s.rdb = redis.NewClient(&redis.Options{})
			defer s.rdb.Close()
			s.rdb.AddHook(hook)
			w := httptest.NewRecorder()
			s.handleEnqueue(w, httptest.NewRequest("POST", "/jobs/enqueue", strings.NewReader(`{"queue":"email","job_type":"send","payload":{"to":"test"}}`)))
			if len(d.queries) != 1 || !strings.Contains(d.queries[0], "INSERT INTO public.np_jobqueue_jobs") {
				t.Fatalf("snapshot queries: %v", d.queries)
			}
			var id pgtype.UUID
			if err := id.Scan(d.args[0][0]); err != nil {
				t.Fatalf("job id is not a Postgres UUID: %v", err)
			}
			if fail {
				if w.Code != 503 || len(hook.commands) != 0 {
					t.Fatalf("status=%d Redis=%v", w.Code, hook.commands)
				}
				return
			}
			if w.Code != 202 || len(hook.commands) != 1 || hook.commands[0] != "lpush" {
				t.Fatalf("status=%d Redis=%v", w.Code, hook.commands)
			}
			var job jobPayload
			if err := json.Unmarshal(hook.args[0][2].([]byte), &job); err != nil {
				t.Fatal(err)
			}
			if job.ID != d.args[0][0] || job.JobType != "send" || d.args[0][1] != "email" || d.args[0][3] != `{"to":"test"}` {
				t.Fatalf("snapshot=%v job=%+v", d.args[0], job)
			}
		})
	}
}

func TestServerUsesMigratedPool(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://test:test@127.0.0.1:1/test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s, err := NewServer(Config{RedisURL: "redis://127.0.0.1:1"}, newTestServer().log, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer s.rdb.Close()
	if s.db != pool {
		t.Fatal("server did not retain migrated pool")
	}
}

func TestHealthCannotBypassRedis(t *testing.T) {
	t.Setenv("JOBQUEUE_HEALTH_IGNORE_REDIS", "true")
	// Supply a bounded, unreachable migration pool so the old bypass fails an
	// assertion instead of panicking when it incorrectly skips Redis.
	pool, err := pgxpool.New(context.Background(), "postgres://test:test@127.0.0.1:1/test?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	previous := bootPool
	bootPool = pool
	defer func() { bootPool = previous }()
	s := newTestServer()
	h := &redisHook{err: errors.New("offline")}
	s.rdb = redis.NewClient(&redis.Options{})
	defer s.rdb.Close()
	s.rdb.AddHook(h)
	w := httptest.NewRecorder()
	s.handleHealth(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 503 || len(h.commands) != 1 || h.commands[0] != "ping" {
		t.Fatalf("status=%d Redis=%v", w.Code, h.commands)
	}
}

func TestExhaustedJobWritesStatusAndDLQ(t *testing.T) {
	s := newTestServer()
	s.cfg.MaxAttempts = 1
	d := &snapshotDB{}
	s.db = d
	h := &redisHook{}
	s.rdb = redis.NewClient(&redis.Options{})
	defer s.rdb.Close()
	s.rdb.AddHook(h)
	s.processJob(context.Background(), "default", "queue:default:processing", `{"id":"00000000-0000-4000-8000-000000000001","job_type":"unknown","payload":{},"attempt_count":0}`)
	if len(d.queries) != 3 || d.args[0][0] != "active" || d.args[1][0] != "dead" || !strings.Contains(d.queries[2], "INSERT INTO public.np_job_dlq") {
		t.Fatalf("queries=%v args=%v", d.queries, d.args)
	}
	for _, q := range d.queries {
		if !strings.Contains(q, "source_account_id='primary'") {
			t.Fatalf("unscoped query: %s", q)
		}
	}
	if len(h.commands) != 1 || h.commands[0] != "lrem" {
		t.Fatalf("Redis commands=%v", h.commands)
	}
}

func TestJobIDFitsSnapshotUUID(t *testing.T) {
	var id pgtype.UUID
	if err := id.Scan(generateID()); err != nil {
		t.Fatal(err)
	}
}

// requeueDB is a mock database that supports QueryRow for handleRequeue tests.
// It records all Exec queries and returns a fixed DLQ entry ID from QueryRow.
type requeueDB struct {
	queries []string
	args    [][]any
	dlqID   string
}

func (d *requeueDB) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	d.queries = append(d.queries, q)
	d.args = append(d.args, args)
	return pgconn.NewCommandTag("UPDATE 0 1"), nil
}

func (*requeueDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected Query")
}

func (d *requeueDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &requeueRow{dlqID: d.dlqID, queue: "email", jobType: "send", payload: json.RawMessage(`{}`)}
}

type requeueRow struct {
	dlqID   string
	queue   string
	jobType string
	payload json.RawMessage
	err     error
}

func (r *requeueRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 4 {
		return errors.New("expected 4 scan destinations")
	}
	switch v := dest[0].(type) {
	case *string:
		*v = r.dlqID
	default:
		return errors.New("dest[0] must be *string")
	}
	switch v := dest[1].(type) {
	case *string:
		*v = r.queue
	default:
		return errors.New("dest[1] must be *string")
	}
	switch v := dest[2].(type) {
	case *string:
		*v = r.jobType
	default:
		return errors.New("dest[2] must be *string")
	}
	switch v := dest[3].(type) {
	case *json.RawMessage:
		*v = r.payload
	default:
		return errors.New("dest[3] must be *json.RawMessage")
	}
	return nil
}

func (r *requeueRow) Err() error { return r.err }

// TestRequeueDeletesSpecificDLQEntry verifies that handleRequeue deletes only
// the specific DLQ row it requeued (by d.id), not all rows matching job_id.
// Regression: with MaxAttempts=1 a fast worker can re-fail and insert a fresh
// DLQ entry before a broad DELETE runs, silently erasing the new failure.
func TestRequeueDeletesSpecificDLQEntry(t *testing.T) {
	s := newTestServer()
	db := &requeueDB{dlqID: "dlq-entry-uuid-001"}
	s.db = db
	hook := &redisHook{}
	s.rdb = redis.NewClient(&redis.Options{})
	defer s.rdb.Close()
	s.rdb.AddHook(hook)

	w := httptest.NewRecorder()
	body := strings.NewReader(`{"job_id":"job-uuid-001"}`)
	s.handleRequeue(w, httptest.NewRequest("POST", "/jobs/dlq/requeue", body))

	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	// Expect exactly 2 Exec calls: UPDATE jobs, then DELETE dlq by specific id.
	if len(db.queries) != 2 {
		t.Fatalf("expected 2 Exec queries, got %d: %v", len(db.queries), db.queries)
	}

	deleteQuery := db.queries[1]
	if !strings.Contains(deleteQuery, "DELETE FROM public.np_job_dlq WHERE id=$1") {
		t.Errorf("DELETE must target specific DLQ entry by id; got: %s", deleteQuery)
	}
	if strings.Contains(deleteQuery, "WHERE job_id=") {
		t.Errorf("DELETE must NOT use broad job_id filter; got: %s", deleteQuery)
	}
	if len(db.args[1]) < 1 || db.args[1][0] != "dlq-entry-uuid-001" {
		t.Errorf("DELETE arg must be the DLQ entry id 'dlq-entry-uuid-001'; got args: %v", db.args[1])
	}
}
