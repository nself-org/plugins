//! Unit tests for the boot-migration runner (no database needed).

use super::*;

fn disk_files() -> Vec<(String, String)> {
    let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("migrations");
    let mut v: Vec<(String, String)> = std::fs::read_dir(dir)
        .unwrap()
        .map(|e| e.unwrap().path())
        .filter(|p| {
            let n = p.file_name().unwrap().to_string_lossy().to_string();
            p.is_file() && n.ends_with(".sql") && !n.ends_with(".down.sql")
        })
        .map(|p| (p.file_name().unwrap().to_string_lossy().to_string(), std::fs::read_to_string(&p).unwrap()))
        .collect();
    v.sort();
    v
}

#[test]
fn embedded_list_equals_the_files_on_disk() {
    let mut emb: Vec<(String, String)> = MIGRATIONS.iter().map(|(n, s)| (n.to_string(), s.to_string())).collect();
    emb.sort();
    assert_eq!(emb, disk_files(), "src/migrate.rs MIGRATIONS drifted from migrations/*.sql");
}

#[test]
fn schema_matches_the_np_name_rule() {
    assert!(SCHEMA.starts_with("np_") && SCHEMA.len() <= 63);
    assert!(SCHEMA.bytes().all(|b| b == b'_' || b.is_ascii_lowercase() || b.is_ascii_digit()));
}

#[test]
fn checksum_is_sha256_hex() {
    assert_eq!(checksum("abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
}

#[test]
fn embedded_files_have_no_transaction_control() {
    for f in files() {
        assert_eq!(txcontrol::find_tx_control(f.sql), None, "{}", f.name);
    }
}

#[test]
fn refuses_a_pending_file_that_sorts_before_an_applied_one() {
    let early = File { name: "001_a.sql", sql: "SELECT 1", sum: checksum("SELECT 1") };
    let rec: HashMap<String, String> = [("002_b.sql".to_string(), "x".to_string())].into_iter().collect();
    assert!(matches!(check_pending(&[&early], &rec), Err(MigrateError::OutOfOrder { .. })));
    assert!(check_pending(&[&early], &HashMap::new()).is_ok());
}

#[test]
fn refuses_transaction_control_in_a_pending_file() {
    let bad = File { name: "002_c.sql", sql: "BEGIN; SELECT 1; COMMIT;", sum: String::new() };
    assert!(matches!(check_pending(&[&bad], &HashMap::new()), Err(MigrateError::File { .. })));
}

#[test]
fn progress_json_reports_drift_only_when_non_zero() {
    assert_eq!(Progress { applied: 1, expected: 1, ..Progress::default() }.to_json(), serde_json::json!({"applied": 1, "expected": 1}));
    assert_eq!(
        Progress { applied: 0, expected: 1, drifted: 1, deleted: 2 }.to_json(),
        serde_json::json!({"applied": 0, "expected": 1, "drifted": 1, "deleted": 2})
    );
}
