CREATE TABLE node_token (jti TEXT PRIMARY KEY, serial TEXT NOT NULL, session TEXT NOT NULL, node_id TEXT NOT NULL, bound_at INTEGER NOT NULL, exp INTEGER NOT NULL);
CREATE INDEX node_token_exp ON node_token(exp);
