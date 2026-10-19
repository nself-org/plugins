CREATE TABLE trust_grant (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 principal TEXT NOT NULL,
 role TEXT NOT NULL CHECK (role IN ('member','release','admin')),
 projects TEXT NOT NULL,
 subject TEXT NOT NULL,
 granted_by TEXT NOT NULL,
 granted_at INTEGER NOT NULL,
 revoked_at INTEGER
);
CREATE UNIQUE INDEX trust_grant_live ON trust_grant(principal) WHERE revoked_at IS NULL;
CREATE TABLE trust_token (
 id TEXT PRIMARY KEY,
 digest TEXT NOT NULL UNIQUE,
 principal TEXT NOT NULL,
 role TEXT NOT NULL CHECK (role IN ('member','release','admin')),
 projects TEXT NOT NULL,
 subject TEXT NOT NULL,
 issued_by TEXT NOT NULL,
 issued_at INTEGER NOT NULL,
 expires_at INTEGER NOT NULL,
 revoked_at INTEGER
);
CREATE TABLE trust_cert_subject (
 serial TEXT PRIMARY KEY,
 principal TEXT NOT NULL,
 subject TEXT NOT NULL
);
