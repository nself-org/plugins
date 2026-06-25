package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
)

func GetJobRuns(pool *pgxpool.Pool, jobID string, limit, offset int) ([]CronRun, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT id, job_id, started_at, completed_at, status, http_status, error, duration_ms, attempt
		 FROM np_cron_runs
		 WHERE job_id = $1
		 ORDER BY started_at DESC
		 LIMIT $2 OFFSET $3`,
		jobID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var runs []CronRun
	for rows.Next() {
		var r CronRun
		if err := rows.Scan(&r.ID, &r.JobID, &r.StartedAt, &r.CompletedAt,
			&r.Status, &r.HTTPStatus, &r.Error, &r.DurationMs, &r.Attempt); err != nil {
			return nil, 0, err
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	var total int64
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM np_cron_runs WHERE job_id = $1`, jobID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	return runs, total, nil
}

// EnvJob represents a cron job declared via CRON_JOB_<N>_* environment variables.
// These are bootstrapped into Postgres on startup so schedule config is
// declared as infrastructure-as-code and survives container restarts + rebuilds.
type EnvJob struct {
	N           int
	Name        string
	Schedule    string
	CallbackURL string
	Payload     *string
}

// LoadEnvJobs reads CRON_JOB_<N>_SCHEDULE and CRON_JOB_<N>_COMMAND (callback URL)
// from the environment for N=1..20. Returns only fully-declared entries (both
// SCHEDULE and COMMAND must be non-empty to be included).
//
// Env var format:
//
//	CRON_JOB_1_SCHEDULE=0 3 * * *
//	CRON_JOB_1_COMMAND=http://myservice:8080/tasks/nightly-backup
//	CRON_JOB_1_NAME=nightly-backup        (optional; defaults to "env-job-1")
//	CRON_JOB_1_PAYLOAD={"bucket":"main"}  (optional; passed as JSON body)
func LoadEnvJobs() []EnvJob {
	var jobs []EnvJob
	for i := 1; i <= 20; i++ {
		n := fmt.Sprintf("%d", i)
		schedule := os.Getenv("CRON_JOB_" + n + "_SCHEDULE")
		command := os.Getenv("CRON_JOB_" + n + "_COMMAND")
		if schedule == "" || command == "" {
			continue
		}
		name := os.Getenv("CRON_JOB_" + n + "_NAME")
		if name == "" {
			name = "env-job-" + n
		}
		var payload *string
		if p := os.Getenv("CRON_JOB_" + n + "_PAYLOAD"); p != "" {
			payload = &p
		}
		jobs = append(jobs, EnvJob{
			N:           i,
			Name:        name,
			Schedule:    schedule,
			CallbackURL: command,
			Payload:     payload,
		})
	}
	return jobs
}

// SeedEnvJobs upserts env-declared jobs into np_cron_jobs using the job name as
// the natural key. If a job with the same name already exists its schedule and
// callback URL are updated so operators can change them by updating env vars and
// restarting the container (no manual DB edits required).
//
// Jobs that previously existed via SeedEnvJobs but are no longer declared in env
// are left untouched (not deleted) — they become regular API-managed jobs.
//
// Returns the number of jobs upserted and any error.
func SeedEnvJobs(pool *pgxpool.Pool) (int, error) {
	jobs := LoadEnvJobs()
	if len(jobs) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var upserted int
	for _, j := range jobs {
		if err := ValidateCronExpr(j.Schedule); err != nil {
			log.Printf("SeedEnvJobs: CRON_JOB_%d_SCHEDULE invalid (%q): %v — skipping", j.N, j.Schedule, err)
			continue
		}

		nextRun := NextRunTime(j.Schedule)

		tag, err := pool.Exec(ctx,
			`INSERT INTO np_cron_jobs (id, name, cron_expr, callback_url, payload, enabled, next_run_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, TRUE, $5)
			 ON CONFLICT (name) DO UPDATE SET
			   cron_expr    = EXCLUDED.cron_expr,
			   callback_url = EXCLUDED.callback_url,
			   payload      = EXCLUDED.payload,
			   next_run_at  = EXCLUDED.next_run_at,
			   enabled      = TRUE`,
			j.Name, j.Schedule, j.CallbackURL, j.Payload, nextRun,
		)
		if err != nil {
			log.Printf("SeedEnvJobs: upsert job %q failed: %v", j.Name, err)
			continue
		}
		if tag.RowsAffected() > 0 {
			upserted++
			log.Printf("SeedEnvJobs: seeded job %q (%s → %s)", j.Name, j.Schedule, j.CallbackURL)
		}
	}
	return upserted, nil
}

// GetDueJobIDs returns IDs of enabled jobs whose next_run_at is in the past.
