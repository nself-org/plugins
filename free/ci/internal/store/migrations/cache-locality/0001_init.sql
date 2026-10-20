CREATE TABLE cache_locality (node TEXT NOT NULL CHECK(length(node)>0), realm TEXT NOT NULL CHECK(length(realm)>0), cache_key TEXT NOT NULL CHECK(length(cache_key)>0), bytes INTEGER NOT NULL CHECK(bytes>=0), last_seen INTEGER NOT NULL, PRIMARY KEY(node,realm,cache_key));
CREATE INDEX cache_locality_entry ON cache_locality(realm,cache_key,node);
CREATE INDEX cache_locality_seen ON cache_locality(last_seen,node);
