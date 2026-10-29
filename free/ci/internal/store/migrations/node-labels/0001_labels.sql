CREATE TABLE node_label (
 node_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL DEFAULT '',
 updated_at INTEGER NOT NULL,
 PRIMARY KEY (node_id, key)
);
CREATE TABLE node_label_audit (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, node_id TEXT NOT NULL, actor TEXT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('set','change','remove')),
 key TEXT NOT NULL, old_value TEXT NOT NULL DEFAULT '', value TEXT NOT NULL DEFAULT '',
 at INTEGER NOT NULL
);
CREATE INDEX node_label_audit_node ON node_label_audit(node_id, seq);
