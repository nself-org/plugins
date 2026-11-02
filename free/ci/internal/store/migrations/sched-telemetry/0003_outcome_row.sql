-- Expand-only (P7-SCHED-20). `nself ci runs stats` reads every outcome of the window, and reading ten columns
-- of each of 100 000 rows cost about 90 ms of CPU (the whole document 580 ms on the hosted runner, over its
-- 500 ms budget) however the statement was written: SQLite decodes a column in about 30 ns and hands each row
-- over in about 1 us. stat_row keeps what the stats read needs of an outcome as one text, set at write time by
-- the trigger below (and for existing rows by the UPDATE): attempt number, wait, runtime, cold start and a flag
-- number (1 passed, 2 infra failure, 4 ephemeral runner), then project, runner, trigger, job key and job id,
-- joined by char(31). stat_row is NULL exactly when the stats read leaves the outcome out: no job (the attempt
-- join found none) or a result other than passed and failed. The partial index answers the read by itself.
-- The format is the one internal/store/sched_stats_fold.go splits; change both together.
ALTER TABLE schedtel_outcome ADD COLUMN stat_row TEXT;
UPDATE schedtel_outcome SET stat_row=printf('%d'||char(31)||'%d'||char(31)||'%d'||char(31)||'%d'||char(31)||'%d'||char(31)||'%s'||char(31)||'%s'||char(31)||'%s'||char(31)||'%s'||char(31)||'%s',attempt_n,wait_ms,runtime_ms,cold_start_ms,(result='passed')+2*(failure_class='infra')+4*(runner_class='ephemeral'),project,runner,pipeline_trigger,job_key,job_id) WHERE job_id IS NOT NULL AND result IN ('passed','failed');
CREATE TRIGGER IF NOT EXISTS schedtel_outcome_row AFTER UPDATE OF job_id,attempt_n,pipeline_trigger ON schedtel_outcome BEGIN UPDATE schedtel_outcome SET stat_row=CASE WHEN NEW.job_id IS NOT NULL AND NEW.result IN ('passed','failed') THEN printf('%d'||char(31)||'%d'||char(31)||'%d'||char(31)||'%d'||char(31)||'%d'||char(31)||'%s'||char(31)||'%s'||char(31)||'%s'||char(31)||'%s'||char(31)||'%s',NEW.attempt_n,NEW.wait_ms,NEW.runtime_ms,NEW.cold_start_ms,(NEW.result='passed')+2*(NEW.failure_class='infra')+4*(NEW.runner_class='ephemeral'),NEW.project,NEW.runner,NEW.pipeline_trigger,NEW.job_key,NEW.job_id) END WHERE rowid=NEW.rowid; END;
CREATE INDEX IF NOT EXISTS schedtel_outcome_row ON schedtel_outcome(project,ts,stat_row) WHERE stat_row IS NOT NULL;
