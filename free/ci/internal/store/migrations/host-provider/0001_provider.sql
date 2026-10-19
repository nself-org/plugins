CREATE TABLE provider_fact (provider TEXT NOT NULL, scope TEXT NOT NULL, key TEXT NOT NULL, value_json BLOB NOT NULL, observed_at INTEGER NOT NULL, PRIMARY KEY(provider, scope, key));
CREATE TABLE provider_mark (name TEXT NOT NULL, key TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY(name, key));
CREATE TABLE dispatch_intent (attempt_id TEXT NOT NULL PRIMARY KEY, provider TEXT NOT NULL, nonce TEXT NOT NULL, run_id TEXT NOT NULL, ref TEXT NOT NULL DEFAULT '', epoch INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL);
