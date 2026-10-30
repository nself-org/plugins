-- Expand-only (P7-SCHED-20). `nself ci runs stats` sums cache use per project; joining cache_attempt for every
-- use row cost a seek a row, so each use keeps its attempt's project, set by triggers at write time. project is
-- NULL exactly when the attempt has no cache_attempt row, which the old LEFT JOIN read as ''.
ALTER TABLE cache_use ADD COLUMN project TEXT;
UPDATE cache_use SET project=(SELECT c.project FROM cache_attempt c WHERE c.attempt_id=cache_use.attempt_id);
CREATE TRIGGER cache_use_project AFTER INSERT ON cache_use BEGIN UPDATE cache_use SET project=(SELECT c.project FROM cache_attempt c WHERE c.attempt_id=NEW.attempt_id) WHERE id=NEW.id; END;
CREATE TRIGGER cache_use_attempt_in AFTER INSERT ON cache_attempt BEGIN UPDATE cache_use SET project=NEW.project WHERE attempt_id=NEW.attempt_id; END;
CREATE TRIGGER cache_use_attempt_out AFTER DELETE ON cache_attempt BEGIN UPDATE cache_use SET project=NULL WHERE attempt_id=OLD.attempt_id; END;
CREATE INDEX cache_use_stats ON cache_use(ts,project,result,bytes);
