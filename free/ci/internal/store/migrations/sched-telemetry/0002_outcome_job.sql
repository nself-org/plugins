-- Expand-only (P7-SCHED-20). `nself ci runs stats` reads every outcome of the window with its job, attempt
-- number and trigger. Joining attempt, job and pipeline for each row cost three index seeks a row (2.5 s for
-- 100 000 outcomes on the Linux runner), so the outcome keeps the three values, set by triggers at write time.
-- job_id is NULL exactly when the attempt-job-pipeline join finds no row; the stats read leaves those out,
-- as the join did. The covering index answers that read from the index alone.
ALTER TABLE schedtel_outcome ADD COLUMN job_id TEXT;
ALTER TABLE schedtel_outcome ADD COLUMN attempt_n INTEGER;
ALTER TABLE schedtel_outcome ADD COLUMN pipeline_trigger TEXT;
UPDATE schedtel_outcome SET (job_id,attempt_n,pipeline_trigger)=(SELECT a.job_id,a.n,p.trigger FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=schedtel_outcome.attempt_id);
CREATE TRIGGER schedtel_outcome_job AFTER INSERT ON schedtel_outcome BEGIN UPDATE schedtel_outcome SET (job_id,attempt_n,pipeline_trigger)=(SELECT a.job_id,a.n,p.trigger FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=NEW.attempt_id) WHERE rowid=NEW.rowid; END;
CREATE TRIGGER schedtel_outcome_attempt_in AFTER INSERT ON attempt BEGIN UPDATE schedtel_outcome SET (job_id,attempt_n,pipeline_trigger)=(SELECT a.job_id,a.n,p.trigger FROM attempt a JOIN job j ON j.id=a.job_id JOIN pipeline p ON p.id=j.pipeline_id WHERE a.id=NEW.id) WHERE attempt_id=NEW.id; END;
CREATE TRIGGER schedtel_outcome_attempt_out AFTER DELETE ON attempt BEGIN UPDATE schedtel_outcome SET job_id=NULL,attempt_n=NULL,pipeline_trigger=NULL WHERE attempt_id=OLD.id; END;
CREATE INDEX schedtel_outcome_stats ON schedtel_outcome(project,ts,job_id,attempt_n,attempt_id,job_key,runner,runner_class,result,failure_class,pipeline_trigger,wait_ms,runtime_ms,cold_start_ms);
