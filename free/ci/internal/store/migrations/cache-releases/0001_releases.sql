CREATE TABLE cache_release (
 run_id TEXT PRIMARY KEY,
 project TEXT NOT NULL,
 revision TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('published','awaiting-signature','failed')),
 manifest_sha256 TEXT NOT NULL DEFAULT '' CHECK(manifest_sha256='' OR length(manifest_sha256)=64),
 key_id TEXT NOT NULL DEFAULT '',
 reason TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL,
 updated_at INTEGER NOT NULL,
 CHECK(state<>'published' OR (manifest_sha256<>'' AND key_id<>''))
);
CREATE INDEX cache_release_project ON cache_release(project, updated_at DESC);
CREATE TRIGGER cache_release_published_immutable BEFORE UPDATE ON cache_release
WHEN OLD.state='published' AND (NEW.state<>'published' OR NEW.manifest_sha256<>OLD.manifest_sha256 OR NEW.key_id<>OLD.key_id OR NEW.project<>OLD.project OR NEW.revision<>OLD.revision)
BEGIN SELECT RAISE(ABORT,'E607 immutable: a published release record does not change'); END;
CREATE TRIGGER cache_release_published_undeletable BEFORE DELETE ON cache_release
WHEN OLD.state='published'
BEGIN SELECT RAISE(ABORT,'E607 immutable: a published release record is not deleted'); END;
