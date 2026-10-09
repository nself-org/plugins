CREATE TABLE node (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, version INTEGER NOT NULL,
 lifecycle TEXT NOT NULL, trust_json TEXT NOT NULL, deploy_host_json TEXT NOT NULL,
 state TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE TABLE capability_snapshot (
 node_id TEXT PRIMARY KEY REFERENCES node(id) ON DELETE CASCADE,
 digest TEXT NOT NULL, doc_json TEXT NOT NULL, observed_at INTEGER NOT NULL
);
