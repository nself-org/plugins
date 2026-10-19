CREATE TABLE excl_project (
 root TEXT PRIMARY KEY, seen_at INTEGER NOT NULL
);
CREATE TABLE excl_entry (
 root TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('host','address','prefix','fingerprint','machine_id')),
 value TEXT NOT NULL, source TEXT NOT NULL CHECK(source IN ('inventory','never-list','marker')),
 PRIMARY KEY (root, kind, value)
);
CREATE TABLE excl_state (
 root TEXT PRIMARY KEY, refreshed_at INTEGER NOT NULL, ok INTEGER NOT NULL CHECK(ok IN (0,1)),
 last_ok_at INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT ''
);
