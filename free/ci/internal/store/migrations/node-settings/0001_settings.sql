CREATE TABLE node_settings (
 node_id TEXT PRIMARY KEY REFERENCES node(id) ON DELETE CASCADE,
 reserved_cpu REAL NOT NULL DEFAULT 0, reserved_mem_mb INTEGER NOT NULL DEFAULT 0,
 interactive TEXT NOT NULL, on_battery TEXT NOT NULL, persistence TEXT NOT NULL,
 version INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
