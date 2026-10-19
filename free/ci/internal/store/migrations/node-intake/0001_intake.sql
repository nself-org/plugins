CREATE TABLE intake (
 seq INTEGER PRIMARY KEY AUTOINCREMENT,
 id TEXT NOT NULL UNIQUE,
 source TEXT NOT NULL, delivery_id TEXT NOT NULL DEFAULT '',
 project TEXT NOT NULL, revision TEXT NOT NULL, trigger_class TEXT NOT NULL, policy_digest TEXT NOT NULL,
 semantic_key TEXT NOT NULL, pipeline TEXT NOT NULL DEFAULT '', facts BLOB NOT NULL,
 webhook INTEGER NOT NULL DEFAULT 0 CHECK(webhook IN (0,1)),
 forced INTEGER NOT NULL DEFAULT 0 CHECK(forced IN (0,1)),
 state TEXT NOT NULL CHECK(state IN ('queued','dispatched','done','failed')),
 claimed_by TEXT NOT NULL DEFAULT '', claim_at INTEGER NOT NULL DEFAULT 0, attempts INTEGER NOT NULL DEFAULT 0,
 pipeline_id TEXT NOT NULL DEFAULT '', result TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX intake_state ON intake(state, seq);
CREATE INDEX intake_semantic ON intake(semantic_key, state);
CREATE TABLE intake_delivery (
 source TEXT NOT NULL, delivery_id TEXT NOT NULL, intake_id TEXT NOT NULL,
 outcome TEXT NOT NULL CHECK(outcome IN ('enqueued','coalesced')), created_at INTEGER NOT NULL,
 PRIMARY KEY (source, delivery_id)
);
CREATE INDEX intake_delivery_age ON intake_delivery(created_at);
CREATE TABLE intake_idem (
 key_digest TEXT PRIMARY KEY, req_digest TEXT NOT NULL, intake_id TEXT NOT NULL, created_at INTEGER NOT NULL
);
