CREATE TABLE coordinator_mode (
 id INTEGER PRIMARY KEY CHECK(id=1),
 mode TEXT NOT NULL CHECK(mode IN ('personal','team')),
 set_at INTEGER NOT NULL
);
CREATE TRIGGER coordinator_mode_no_downgrade BEFORE UPDATE ON coordinator_mode
WHEN OLD.mode='team' AND NEW.mode<>'team'
BEGIN SELECT RAISE(ABORT,'coordinator mode never returns to personal'); END;
CREATE TRIGGER coordinator_mode_keep BEFORE DELETE ON coordinator_mode
WHEN OLD.mode='team'
BEGIN SELECT RAISE(ABORT,'coordinator mode never returns to personal'); END;
