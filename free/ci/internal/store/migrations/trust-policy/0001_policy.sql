CREATE TABLE trust_policy_row (
 scope TEXT NOT NULL CHECK (scope IN ('user','team','project')),
 subject TEXT NOT NULL, key TEXT NOT NULL, value_json TEXT NOT NULL,
 updated_by TEXT NOT NULL, updated_at INTEGER NOT NULL,
 PRIMARY KEY (scope, subject, key)
);
CREATE TABLE trust_policy_pin (
 subject TEXT PRIMARY KEY, sha TEXT NOT NULL,
 pinned_by TEXT NOT NULL, pinned_at INTEGER NOT NULL
);
