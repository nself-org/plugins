//! Boot-time plugin migrations (contract plugin.boot-migrations v1, ADR 0010).
//!
//! Rust mirror of cli sdk/go/migrate (strict mode). `apply` runs once from main() before the
//! service serves traffic:
//!
//! - files are the embedded `migrations/*.sql` (no `*.down.sql`), in byte order of the name;
//! - a session advisory lock on `hashtext('np_media_processing.schema_migrations')` serialises
//!   replicas, held on a dedicated connection for the whole run;
//! - one transaction per file: SET LOCAL lock_timeout / statement_timeout / search_path, the file's
//!   SQL, and its ledger row commit together or not at all;
//! - the ledger is `np_media_processing.schema_migrations(filename, checksum, applied_at)`; an
//!   applied file whose sha256 changed is fatal; a pending file sorting before an applied one is
//!   refused; transaction control inside a file is refused before anything runs;
//! - `status` feeds /health `"migrations": {"applied": N, "expected": M}`.
//!
//! The embedded list is compared with the files on disk by a unit test, so it cannot drift.

mod txcontrol;

use deadpool_postgres::Pool;
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use std::fmt;
use std::time::Duration;
use tokio_postgres::{Client, NoTls};

/// The plugin's own schema (manifest `schema`), np_<name with - replaced by _>.
pub const SCHEMA: &str = "np_media_processing";
const LOCK_KEY: &str = "np_media_processing.schema_migrations";
const LOCK_TIMEOUT_MS: u64 = 5_000;
const STATEMENT_TIMEOUT_MS: u64 = 60_000;
const LOCK_WAIT: Duration = Duration::from_secs(120);

/// Embedded migrations: (file name, SQL). Keep in step with migrations/*.sql (unit-tested).
pub const MIGRATIONS: &[(&str, &str)] = &[("001_init.sql", include_str!("../migrations/001_init.sql"))];

#[derive(Debug)]
pub enum MigrateError {
    Db(String),
    File { file: String, msg: String },
    Checksum { file: String, recorded: String, now: String },
    OutOfOrder { file: String, after: String },
    Unledgered,
    LegacyLedger,
}

impl fmt::Display for MigrateError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Db(m) => write!(f, "migrate: {m}"),
            Self::File { file, msg } => write!(f, "migrate: file {file}: {msg}"),
            Self::Checksum { file, recorded, now } => write!(
                f,
                "migrate: file {file} was applied with checksum {recorded} but now has {now}: an applied migration must never be edited, add a new file instead"
            ),
            Self::OutOfOrder { file, after } => write!(
                f,
                "migrate: file {file} sorts before {after}, which is already applied: give the new migration a name that sorts after every applied file"
            ),
            Self::Unledgered => write!(f, "migrate: schema {SCHEMA} has objects but no ledger; adopt it with a baseline before booting this version"),
            Self::LegacyLedger => write!(f, "migrate: legacy ledger without a checksum column in {SCHEMA}.schema_migrations"),
        }
    }
}

impl std::error::Error for MigrateError {}

fn db<E: fmt::Display>(what: &str) -> impl Fn(E) -> MigrateError + '_ {
    move |e| MigrateError::Db(format!("{what}: {e}"))
}

/// Migration progress served in /health (`drifted` / `deleted` only when non-zero).
#[derive(Debug, Default, PartialEq)]
pub struct Progress {
    pub applied: usize,
    pub expected: usize,
    pub drifted: usize,
    pub deleted: usize,
}

impl Progress {
    pub fn to_json(&self) -> serde_json::Value {
        let mut m = serde_json::json!({"applied": self.applied, "expected": self.expected});
        if self.drifted > 0 {
            m["drifted"] = self.drifted.into();
        }
        if self.deleted > 0 {
            m["deleted"] = self.deleted.into();
        }
        m
    }
}

struct File {
    name: &'static str,
    sql: &'static str,
    sum: String,
}

/// The embedded files in byte order of name, with their sha256 checksum.
fn files() -> Vec<File> {
    let mut v: Vec<File> = MIGRATIONS
        .iter()
        .map(|(name, sql)| File { name, sql, sum: checksum(sql) })
        .collect();
    v.sort_by(|a, b| a.name.as_bytes().cmp(b.name.as_bytes()));
    v
}

fn checksum(sql: &str) -> String {
    Sha256::digest(sql.as_bytes()).iter().map(|b| format!("{b:02x}")).collect()
}

fn ledger() -> String {
    format!("\"{SCHEMA}\".schema_migrations")
}

/// Applies every pending embedded migration and returns the names it applied.
pub async fn apply(database_url: &str) -> Result<Vec<String>, MigrateError> {
    let files = files();
    // A dedicated connection (not from the pool) holds the advisory lock; closing it releases it.
    let (client, conn) = tokio_postgres::connect(database_url, NoTls).await.map_err(db("connect for advisory lock"))?;
    let task = tokio::spawn(async move {
        let _ = conn.await;
    });
    let res = run(&client, &files).await;
    drop(client);
    let _ = task.await;
    res
}

async fn run(client: &Client, files: &[File]) -> Result<Vec<String>, MigrateError> {
    // A role or database default timeout would cut a legitimate wait behind another replica short.
    client
        .batch_execute("SELECT set_config('lock_timeout', '0', false), set_config('statement_timeout', '0', false)")
        .await
        .map_err(db("prepare lock connection"))?;
    tokio::time::timeout(LOCK_WAIT, client.execute("SELECT pg_advisory_lock(hashtext($1))", &[&LOCK_KEY]))
        .await
        .map_err(|_| MigrateError::Db(format!("advisory lock {LOCK_KEY}: timed out after {}s", LOCK_WAIT.as_secs())))?
        .map_err(db("advisory lock"))?;
    ensure_ledger(client).await?;
    let recorded = load_ledger(client).await?;
    let mut pending = Vec::new();
    for f in files {
        match recorded.get(f.name) {
            None => pending.push(f),
            Some(sum) if *sum != f.sum => {
                return Err(MigrateError::Checksum { file: f.name.into(), recorded: sum.clone(), now: f.sum.clone() })
            }
            Some(_) => {}
        }
    }
    check_pending(&pending, &recorded)?;
    let mut applied = Vec::new();
    for f in pending {
        apply_file(client, f).await.map_err(|e| MigrateError::File { file: f.name.into(), msg: e })?;
        applied.push(f.name.to_string());
    }
    Ok(applied)
}

/// Refuses a pending set that must not run: a file sorting before the last applied one, and any
/// file with transaction control. Runs before the first statement, so a refusal changes nothing.
fn check_pending(pending: &[&File], recorded: &HashMap<String, String>) -> Result<(), MigrateError> {
    let last = recorded.keys().max().map(String::as_str).unwrap_or("");
    for f in pending {
        if f.name < last {
            return Err(MigrateError::OutOfOrder { file: f.name.into(), after: last.into() });
        }
    }
    for f in pending {
        if let Some((w, line)) = txcontrol::find_tx_control(f.sql) {
            return Err(MigrateError::File {
                file: f.name.into(),
                msg: format!("migration file contains transaction control; remove it, the runner wraps every file in its own transaction: {w} at line {line}"),
            });
        }
    }
    Ok(())
}

async fn ensure_ledger(client: &Client) -> Result<(), MigrateError> {
    let q = ledger();
    let exists: bool = client.query_one("SELECT to_regclass($1) IS NOT NULL", &[&q]).await.map_err(db("check ledger"))?.get(0);
    if exists {
        let ok: bool = client
            .query_one(
                "SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = to_regclass($1) AND attname = 'checksum' AND NOT attisdropped)",
                &[&q],
            )
            .await
            .map_err(db("inspect ledger"))?
            .get(0);
        return if ok { Ok(()) } else { Err(MigrateError::LegacyLedger) };
    }
    let have_schema: bool = client
        .query_one("SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", &[&SCHEMA])
        .await
        .map_err(db("check schema"))?
        .get(0);
    if have_schema {
        let objs: bool = client
            .query_one(
                "SELECT EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1)
                 OR EXISTS (SELECT 1 FROM pg_proc f JOIN pg_namespace n ON n.oid = f.pronamespace WHERE n.nspname = $1)
                 OR EXISTS (SELECT 1 FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = $1 AND t.typtype IN ('e', 'd'))",
                &[&SCHEMA],
            )
            .await
            .map_err(db("inspect schema"))?
            .get(0);
        if objs {
            return Err(MigrateError::Unledgered);
        }
    } else {
        client.batch_execute(&format!("CREATE SCHEMA \"{SCHEMA}\"")).await.map_err(db("create schema"))?;
    }
    client
        .batch_execute(&format!(
            "CREATE TABLE {q} (filename text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())"
        ))
        .await
        .map_err(db("create ledger"))
}

async fn load_ledger(client: &Client) -> Result<HashMap<String, String>, MigrateError> {
    let rows = client
        .query(&format!("SELECT filename, checksum FROM {}", ledger()), &[])
        .await
        .map_err(db("read ledger"))?;
    Ok(rows.iter().map(|r| (r.get::<_, String>(0), r.get::<_, String>(1))).collect())
}

/// Runs one file and records it in one transaction; any error rolls everything back.
async fn apply_file(client: &Client, f: &File) -> Result<(), String> {
    let marker = "np.migrate_marker";
    let pin = format!(
        "BEGIN; SELECT set_config('lock_timeout', '{LOCK_TIMEOUT_MS}ms', true), set_config('statement_timeout', '{STATEMENT_TIMEOUT_MS}ms', true), \
         set_config('search_path', '\"{SCHEMA}\"', true), set_config('{marker}', '1', true)"
    );
    let body = async {
        client.batch_execute(&pin).await.map_err(|e| format!("pin session settings: {e}"))?;
        if !f.sql.trim().is_empty() {
            client.batch_execute(f.sql).await.map_err(|e| e.to_string())?;
            // Defence in depth behind find_tx_control: a file that ended its own transaction loses
            // the transaction-local marker, and its effects are already committed.
            let still: bool = client
                .query_one(&format!("SELECT current_setting('{marker}', true) = '1'"), &[])
                .await
                .map_err(|e| e.to_string())?
                .get(0);
            if !still {
                return Err("migration file ended its own transaction, so its effects are committed without a ledger row".to_string());
            }
        }
        client
            .execute(&format!("INSERT INTO {} (filename, checksum) VALUES ($1, $2)", ledger()), &[&f.name, &f.sum])
            .await
            .map_err(|e| format!("record in ledger: {e}"))?;
        client.batch_execute("COMMIT").await.map_err(|e| format!("commit: {e}"))
    };
    match body.await {
        Ok(()) => Ok(()),
        Err(e) => {
            let _ = client.batch_execute("ROLLBACK").await;
            Err(e)
        }
    }
}

/// Reads the ledger without locking or writing. A missing ledger counts as zero applied.
pub async fn status(pool: &Pool) -> Result<Progress, MigrateError> {
    let files = files();
    let mut st = Progress { expected: files.len(), ..Progress::default() };
    let client = pool.get().await.map_err(db("status connection"))?;
    let q = ledger();
    let exists: bool = client.query_one("SELECT to_regclass($1) IS NOT NULL", &[&q]).await.map_err(db("check ledger"))?.get(0);
    if !exists {
        return Ok(st);
    }
    let recorded = load_ledger(&client).await?;
    for f in &files {
        match recorded.get(f.name) {
            None => {}
            Some(sum) if *sum != f.sum => st.drifted += 1,
            Some(_) => st.applied += 1,
        }
    }
    st.deleted = recorded.keys().filter(|n| !files.iter().any(|f| f.name == n.as_str())).count();
    Ok(st)
}

#[cfg(test)]
mod tests;
