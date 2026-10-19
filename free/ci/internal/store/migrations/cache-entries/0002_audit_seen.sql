CREATE TABLE cache_audit_seen (action TEXT NOT NULL, target TEXT NOT NULL, detail TEXT NOT NULL, ts INTEGER NOT NULL, PRIMARY KEY(action, target, detail)) WITHOUT ROWID;
CREATE INDEX cache_audit_seen_ts ON cache_audit_seen(ts);
