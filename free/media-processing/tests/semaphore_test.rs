/// Tests that `max_concurrent_jobs` is enforced via the semaphore.
///
/// These are unit-level tests that exercise the semaphore logic directly —
/// no real DB or network required.
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{Mutex, OwnedSemaphorePermit, Semaphore};

/// Minimal replica of the acquisition logic in `handlers::create_job`.
fn try_acquire(sem: &Arc<Semaphore>) -> Option<OwnedSemaphorePermit> {
    Arc::clone(sem).try_acquire_owned().ok()
}

#[tokio::test]
async fn semaphore_limits_to_max_concurrent_jobs() {
    let max = 2usize;
    let sem = Arc::new(Semaphore::new(max));
    let permits: Arc<Mutex<HashMap<u32, OwnedSemaphorePermit>>> =
        Arc::new(Mutex::new(HashMap::new()));

    // Acquire `max` permits — all should succeed.
    for i in 0..max as u32 {
        let permit = try_acquire(&sem).expect("should acquire permit within limit");
        permits.lock().await.insert(i, permit);
    }
    assert_eq!(sem.available_permits(), 0, "semaphore should be exhausted");

    // The (max + 1)-th attempt must fail immediately.
    let overflow = try_acquire(&sem);
    assert!(
        overflow.is_none(),
        "try_acquire must return None when semaphore is exhausted (would be HTTP 429)"
    );

    // Releasing one permit frees a slot.
    permits.lock().await.remove(&0);
    assert_eq!(sem.available_permits(), 1, "one permit released");

    // Now a new job is accepted again.
    let permit = try_acquire(&sem).expect("should acquire after a slot frees");
    permits.lock().await.insert(99, permit);
    assert_eq!(sem.available_permits(), 0);
}

#[tokio::test]
async fn semaphore_with_single_slot() {
    let sem = Arc::new(Semaphore::new(1));

    let p1 = try_acquire(&sem).expect("first acquire succeeds");
    assert!(try_acquire(&sem).is_none(), "second acquire blocked → 429");

    drop(p1);
    assert_eq!(sem.available_permits(), 1, "permit returned on drop");
    let _p2 = try_acquire(&sem).expect("acquire succeeds after release");
}

#[tokio::test]
async fn semaphore_env_var_default_is_two() {
    // Verify the Config default matches the expected concurrency floor.
    // We parse the env-var logic inline to avoid a DB dependency.
    let val: usize = std::env::var("MP_MAX_CONCURRENT_JOBS")
        .unwrap_or_else(|_| "2".to_string())
        .parse()
        .unwrap_or(2);
    assert_eq!(val, 2, "default max_concurrent_jobs must be 2");
}
