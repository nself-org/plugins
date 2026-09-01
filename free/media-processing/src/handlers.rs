use crate::models::*;
use actix_web::{web, HttpRequest, HttpResponse};
use deadpool_postgres::Pool;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{Mutex, OwnedSemaphorePermit, Semaphore};
use uuid::Uuid;

/// Holds one semaphore permit per in-flight job.
/// Permits are acquired on `create_job` and released on `cancel_job` or
/// when a background worker marks the job terminal (completed / failed).
pub struct JobPermits(pub Mutex<HashMap<Uuid, OwnedSemaphorePermit>>);

fn sa(req: &HttpRequest) -> &str {
    req.headers()
        .get("X-Source-Account-ID")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("primary")
}

pub async fn health() -> HttpResponse {
    HttpResponse::Ok().json(serde_json::json!({"status": "ok"}))
}

pub async fn ready(pool: web::Data<Arc<Pool>>) -> HttpResponse {
    match pool.get().await {
        Ok(_) => HttpResponse::Ok().json(serde_json::json!({"status": "ready"})),
        Err(_) => {
            HttpResponse::ServiceUnavailable().json(serde_json::json!({"status": "not ready"}))
        }
    }
}

pub async fn list_profiles(req: HttpRequest, pool: web::Data<Arc<Pool>>) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query(
        "SELECT id, source_account_id, name, description, is_default, video_codec, audio_codec, container, bitrate_kbps, width, height, created_at, updated_at FROM np_mediap_encoding_profiles WHERE source_account_id = $1 ORDER BY created_at DESC",
        &[&sa],
    ).await {
        Ok(rows) => {
            let profiles: Vec<serde_json::Value> = rows.iter().map(|r| serde_json::json!({
                "id": r.get::<_, Uuid>(0).to_string(),
                "source_account_id": r.get::<_, String>(1),
                "name": r.get::<_, String>(2),
                "description": r.get::<_, Option<String>>(3),
                "is_default": r.get::<_, bool>(4),
                "video_codec": r.get::<_, Option<String>>(5),
                "audio_codec": r.get::<_, Option<String>>(6),
                "container": r.get::<_, Option<String>>(7),
                "bitrate_kbps": r.get::<_, Option<i32>>(8),
                "width": r.get::<_, Option<i32>>(9),
                "height": r.get::<_, Option<i32>>(10),
            })).collect();
            HttpResponse::Ok().json(profiles)
        }
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn create_profile(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    body: web::Json<CreateProfileInput>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    let id = Uuid::new_v4();
    match client.execute(
        "INSERT INTO np_mediap_encoding_profiles (id, source_account_id, name, description, is_default, video_codec, audio_codec, container, bitrate_kbps, width, height) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)",
        &[&id, &sa, &body.name, &body.description, &body.is_default.unwrap_or(false), &body.video_codec, &body.audio_codec, &body.container, &body.bitrate_kbps, &body.width, &body.height],
    ).await {
        Ok(_) => HttpResponse::Created().json(serde_json::json!({"id": id.to_string()})),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn get_profile(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    path: web::Path<String>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query_opt(
        "SELECT id, source_account_id, name, description, is_default, video_codec, audio_codec FROM np_mediap_encoding_profiles WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await {
        Ok(Some(r)) => HttpResponse::Ok().json(serde_json::json!({
            "id": r.get::<_, Uuid>(0).to_string(),
            "source_account_id": r.get::<_, String>(1),
            "name": r.get::<_, String>(2),
            "description": r.get::<_, Option<String>>(3),
            "is_default": r.get::<_, bool>(4),
            "video_codec": r.get::<_, Option<String>>(5),
            "audio_codec": r.get::<_, Option<String>>(6),
        })),
        Ok(None) => HttpResponse::NotFound().finish(),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn update_profile(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    path: web::Path<String>,
    body: web::Json<UpdateProfileInput>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    if let Some(name) = &body.name {
        let _ = client.execute(
            "UPDATE np_mediap_encoding_profiles SET name=$1, updated_at=NOW() WHERE id=$2 AND source_account_id=$3",
            &[name, &id, &sa],
        ).await;
    }
    HttpResponse::Ok().json(serde_json::json!({"id": id.to_string(), "updated": true}))
}

pub async fn delete_profile(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    path: web::Path<String>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client
        .execute(
            "DELETE FROM np_mediap_encoding_profiles WHERE id=$1 AND source_account_id=$2",
            &[&id, &sa],
        )
        .await
    {
        Ok(_) => HttpResponse::NoContent().finish(),
        Err(e) => {
            HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()}))
        }
    }
}

pub async fn list_jobs(req: HttpRequest, pool: web::Data<Arc<Pool>>) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query(
        "SELECT id, source_account_id, profile_id, input_url, output_path, status, priority, error_message, created_at FROM np_mediap_jobs WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100",
        &[&sa],
    ).await {
        Ok(rows) => {
            let jobs: Vec<serde_json::Value> = rows.iter().map(|r| serde_json::json!({
                "id": r.get::<_, Uuid>(0).to_string(),
                "source_account_id": r.get::<_, String>(1),
                "profile_id": r.get::<_, Option<Uuid>>(2).map(|u| u.to_string()),
                "input_url": r.get::<_, String>(3),
                "output_path": r.get::<_, Option<String>>(4),
                "status": r.get::<_, String>(5),
                "priority": r.get::<_, i32>(6),
                "error_message": r.get::<_, Option<String>>(7),
            })).collect();
            HttpResponse::Ok().json(jobs)
        }
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn create_job(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    semaphore: web::Data<Arc<Semaphore>>,
    permits: web::Data<Arc<JobPermits>>,
    body: web::Json<CreateJobInput>,
) -> HttpResponse {
    // Gate: try to acquire a slot without waiting.
    // Returns 429 immediately when max_concurrent_jobs is reached.
    // Clone the Arc so try_acquire_owned can consume it by value.
    let permit = match Arc::clone(semaphore.get_ref()).try_acquire_owned() {
        Ok(p) => p,
        Err(_) => {
            return HttpResponse::TooManyRequests().json(serde_json::json!({
                "error": "server busy",
                "detail": "maximum concurrent jobs reached; retry later"
            }));
        }
    };

    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    let id = Uuid::new_v4();
    match client.execute(
        "INSERT INTO np_mediap_jobs (id, source_account_id, profile_id, input_url, output_path, status, priority) VALUES ($1,$2,$3,$4,$5,'pending',$6)",
        &[&id, &sa, &body.profile_id, &body.input_url, &body.output_path, &body.priority.unwrap_or(5)],
    ).await {
        Ok(_) => {
            // Store the permit so it is held for the lifetime of this job.
            permits.0.lock().await.insert(id, permit);
            HttpResponse::Created().json(serde_json::json!({"id": id.to_string(), "status": "pending"}))
        }
        Err(e) => {
            // permit drops here, freeing the slot
            HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()}))
        }
    }
}

pub async fn get_job(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    path: web::Path<String>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query_opt(
        "SELECT id, source_account_id, profile_id, input_url, status, priority, error_message, created_at FROM np_mediap_jobs WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await {
        Ok(Some(r)) => HttpResponse::Ok().json(serde_json::json!({
            "id": r.get::<_, Uuid>(0).to_string(),
            "source_account_id": r.get::<_, String>(1),
            "profile_id": r.get::<_, Option<Uuid>>(2).map(|u| u.to_string()),
            "input_url": r.get::<_, String>(3),
            "status": r.get::<_, String>(4),
            "priority": r.get::<_, i32>(5),
            "error_message": r.get::<_, Option<String>>(6),
        })),
        Ok(None) => HttpResponse::NotFound().finish(),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn cancel_job(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    permits: web::Data<Arc<JobPermits>>,
    path: web::Path<String>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.execute(
        "UPDATE np_mediap_jobs SET status='cancelled', updated_at=NOW() WHERE id=$1 AND source_account_id=$2 AND status IN ('pending','processing')",
        &[&id, &sa],
    ).await {
        Ok(_) => {
            // Release the concurrency slot so new jobs can be accepted.
            permits.0.lock().await.remove(&id);
            HttpResponse::Ok().json(serde_json::json!({"id": id.to_string(), "status": "cancelled"}))
        }
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn list_job_outputs(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    path: web::Path<String>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let job_id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query(
        "SELECT id, job_id, source_account_id, output_type, url, size_bytes, created_at FROM np_mediap_job_outputs WHERE job_id=$1 AND source_account_id=$2 ORDER BY created_at",
        &[&job_id, &sa],
    ).await {
        Ok(rows) => {
            let outputs: Vec<serde_json::Value> = rows.iter().map(|r| serde_json::json!({
                "id": r.get::<_, Uuid>(0).to_string(),
                "job_id": r.get::<_, Uuid>(1).to_string(),
                "source_account_id": r.get::<_, String>(2),
                "output_type": r.get::<_, String>(3),
                "url": r.get::<_, String>(4),
                "size_bytes": r.get::<_, Option<i64>>(5),
            })).collect();
            HttpResponse::Ok().json(outputs)
        }
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn create_upload(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    body: web::Json<CreateUploadInput>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    let id = Uuid::new_v4();
    match client.execute(
        "INSERT INTO np_mediap_uploads (id, source_account_id, filename, file_url, file_size, content_type, status) VALUES ($1,$2,$3,$4,$5,$6,'pending')",
        &[&id, &sa, &body.filename, &body.file_url, &body.file_size, &body.content_type],
    ).await {
        Ok(_) => HttpResponse::Created().json(serde_json::json!({"id": id.to_string()})),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn get_upload(
    req: HttpRequest,
    pool: web::Data<Arc<Pool>>,
    path: web::Path<String>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let id = match Uuid::parse_str(&path) {
        Ok(u) => u,
        Err(_) => {
            return HttpResponse::BadRequest().json(serde_json::json!({"error": "invalid id"}))
        }
    };
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query_opt(
        "SELECT id, source_account_id, filename, file_url, file_size, content_type, status, created_at FROM np_mediap_uploads WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await {
        Ok(Some(r)) => HttpResponse::Ok().json(serde_json::json!({
            "id": r.get::<_, Uuid>(0).to_string(),
            "source_account_id": r.get::<_, String>(1),
            "filename": r.get::<_, String>(2),
            "file_url": r.get::<_, String>(3),
            "file_size": r.get::<_, Option<i64>>(4),
            "content_type": r.get::<_, Option<String>>(5),
            "status": r.get::<_, String>(6),
        })),
        Ok(None) => HttpResponse::NotFound().finish(),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn get_stats(req: HttpRequest, pool: web::Data<Arc<Pool>>) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query_one(
        "SELECT COUNT(*) FILTER (WHERE status='pending') as pending, COUNT(*) FILTER (WHERE status='processing') as processing, COUNT(*) FILTER (WHERE status='completed') as completed, COUNT(*) FILTER (WHERE status='failed') as failed FROM np_mediap_jobs WHERE source_account_id=$1",
        &[&sa],
    ).await {
        Ok(r) => HttpResponse::Ok().json(serde_json::json!({
            "jobs": {
                "pending": r.get::<_, i64>(0),
                "processing": r.get::<_, i64>(1),
                "completed": r.get::<_, i64>(2),
                "failed": r.get::<_, i64>(3),
            }
        })),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}
