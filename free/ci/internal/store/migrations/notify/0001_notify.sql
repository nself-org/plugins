CREATE TABLE notify_sent (repo TEXT NOT NULL, sha TEXT NOT NULL, context TEXT NOT NULL, kind TEXT NOT NULL, created_at INTEGER NOT NULL, PRIMARY KEY(repo, sha, context, kind));
CREATE TABLE notify_last (repo TEXT NOT NULL, ref TEXT NOT NULL, context TEXT NOT NULL, result TEXT NOT NULL, sha TEXT NOT NULL, updated_at INTEGER NOT NULL, PRIMARY KEY(repo, ref, context));
CREATE TABLE notify_digest (day TEXT PRIMARY KEY, sent_at INTEGER NOT NULL);
CREATE TABLE notify_sink (sink TEXT PRIMARY KEY, delivered INTEGER NOT NULL DEFAULT 0, failed INTEGER NOT NULL DEFAULT 0, last_failed_at INTEGER);
CREATE INDEX notify_sent_age ON notify_sent(created_at);
CREATE INDEX notify_last_age ON notify_last(updated_at);
