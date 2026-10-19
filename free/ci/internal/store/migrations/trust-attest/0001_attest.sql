CREATE TABLE attest_use (
 attempt_id TEXT NOT NULL,
 key_id TEXT NOT NULL,
 sig_digest TEXT NOT NULL,
 node_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 repo_id TEXT NOT NULL,
 revision TEXT NOT NULL,
 job TEXT NOT NULL DEFAULT '',
 used_at INTEGER NOT NULL,
 PRIMARY KEY(attempt_id, key_id, sig_digest)
);
CREATE INDEX attest_use_sig ON attest_use(key_id, sig_digest);
CREATE INDEX attest_use_key ON attest_use(key_id);
CREATE TABLE attest_revoked (
 key_id TEXT PRIMARY KEY,
 node_id TEXT NOT NULL,
 reason TEXT NOT NULL,
 revoked_at INTEGER NOT NULL
);
CREATE TABLE attest_quarantine (
 run_id TEXT NOT NULL,
 key_id TEXT NOT NULL,
 quarantined_at INTEGER NOT NULL,
 PRIMARY KEY(run_id, key_id)
);
