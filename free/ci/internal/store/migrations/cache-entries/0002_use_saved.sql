CREATE TABLE cache_use_v2 (id INTEGER PRIMARY KEY AUTOINCREMENT, attempt_id TEXT NOT NULL, spec TEXT NOT NULL, cache_key TEXT NOT NULL, result TEXT NOT NULL CHECK(result IN ('hit','miss','disabled','readonly','denied','refused','saved')), bytes INTEGER NOT NULL CHECK(bytes>=0), ms INTEGER NOT NULL CHECK(ms>=0), ts INTEGER NOT NULL);
INSERT INTO cache_use_v2(id,attempt_id,spec,cache_key,result,bytes,ms,ts) SELECT id,attempt_id,spec,cache_key,result,bytes,ms,ts FROM cache_use;
DROP TABLE cache_use;
ALTER TABLE cache_use_v2 RENAME TO cache_use;
CREATE INDEX cache_use_ts ON cache_use(ts);
CREATE INDEX cache_use_attempt ON cache_use(attempt_id, id);
