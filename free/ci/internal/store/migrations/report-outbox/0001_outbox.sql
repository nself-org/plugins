CREATE TABLE outbox (revision TEXT NOT NULL, context TEXT NOT NULL, seq INTEGER NOT NULL, state TEXT NOT NULL, payload_json BLOB, created_at INTEGER NOT NULL, PRIMARY KEY(revision, context, seq));
CREATE INDEX outbox_unposted ON outbox(state, created_at);
