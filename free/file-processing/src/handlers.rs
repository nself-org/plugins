use crate::models::*;
use actix_web::{web, HttpRequest, HttpResponse};
use deadpool_postgres::Pool;
use std::sync::Arc;
use uuid::Uuid;

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

pub async fn list_jobs(req: HttpRequest, pool: web::Data<Arc<Pool>>) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query(
        "SELECT id, source_account_id, file_key, file_url, content_type, file_size, status, error_message, created_at FROM np_fileproc_jobs WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100",
        &[&sa],
    ).await {
        Ok(rows) => {
            let jobs: Vec<serde_json::Value> = rows.iter().map(|r| serde_json::json!({
                "id": r.get::<_, Uuid>(0).to_string(),
                "source_account_id": r.get::<_, String>(1),
                "file_key": r.get::<_, String>(2),
                "file_url": r.get::<_, String>(3),
                "content_type": r.get::<_, Option<String>>(4),
                "file_size": r.get::<_, Option<i64>>(5),
                "status": r.get::<_, String>(6),
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
    body: web::Json<CreateJobInput>,
) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    let id = Uuid::new_v4();
    match client.execute(
        "INSERT INTO np_fileproc_jobs (id, source_account_id, file_key, file_url, content_type, file_size, status) VALUES ($1,$2,$3,$4,$5,$6,'pending')",
        &[&id, &sa, &body.file_key, &body.file_url, &body.content_type, &body.file_size],
    ).await {
        Ok(_) => HttpResponse::Created().json(serde_json::json!({"id": id.to_string(), "status": "pending"})),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
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
        "SELECT id, source_account_id, file_key, file_url, content_type, file_size, status, error_message FROM np_fileproc_jobs WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await {
        Ok(Some(r)) => HttpResponse::Ok().json(serde_json::json!({
            "id": r.get::<_, Uuid>(0).to_string(),
            "source_account_id": r.get::<_, String>(1),
            "file_key": r.get::<_, String>(2),
            "file_url": r.get::<_, String>(3),
            "content_type": r.get::<_, Option<String>>(4),
            "file_size": r.get::<_, Option<i64>>(5),
            "status": r.get::<_, String>(6),
            "error_message": r.get::<_, Option<String>>(7),
        })),
        Ok(None) => HttpResponse::NotFound().finish(),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn cancel_job(
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
    let _ = client.execute(
        "UPDATE np_fileproc_jobs SET status='cancelled', updated_at=NOW() WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await;
    HttpResponse::NoContent().finish()
}

pub async fn list_thumbnails(req: HttpRequest, pool: web::Data<Arc<Pool>>) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query(
        "SELECT id, job_id, source_account_id, size, url, width, height, file_size, created_at FROM np_fileproc_thumbnails WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100",
        &[&sa],
    ).await {
        Ok(rows) => {
            let items: Vec<serde_json::Value> = rows.iter().map(|r| serde_json::json!({
                "id": r.get::<_, Uuid>(0).to_string(),
                "job_id": r.get::<_, Uuid>(1).to_string(),
                "source_account_id": r.get::<_, String>(2),
                "size": r.get::<_, i32>(3),
                "url": r.get::<_, String>(4),
                "width": r.get::<_, Option<i32>>(5),
                "height": r.get::<_, Option<i32>>(6),
                "file_size": r.get::<_, Option<i64>>(7),
            })).collect();
            HttpResponse::Ok().json(items)
        }
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn get_thumbnail(
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
        "SELECT id, job_id, source_account_id, size, url, width, height FROM np_fileproc_thumbnails WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await {
        Ok(Some(r)) => HttpResponse::Ok().json(serde_json::json!({
            "id": r.get::<_, Uuid>(0).to_string(),
            "job_id": r.get::<_, Uuid>(1).to_string(),
            "source_account_id": r.get::<_, String>(2),
            "size": r.get::<_, i32>(3),
            "url": r.get::<_, String>(4),
            "width": r.get::<_, Option<i32>>(5),
            "height": r.get::<_, Option<i32>>(6),
        })),
        Ok(None) => HttpResponse::NotFound().finish(),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn list_metadata(req: HttpRequest, pool: web::Data<Arc<Pool>>) -> HttpResponse {
    let sa = sa(&req).to_string();
    let client = match pool.get().await {
        Ok(c) => c,
        Err(_) => return HttpResponse::InternalServerError().finish(),
    };
    match client.query(
        "SELECT id, job_id, source_account_id, file_key, mime_type, width, height, duration_ms, created_at FROM np_fileproc_metadata WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 100",
        &[&sa],
    ).await {
        Ok(rows) => {
            let items: Vec<serde_json::Value> = rows.iter().map(|r| serde_json::json!({
                "id": r.get::<_, Uuid>(0).to_string(),
                "job_id": r.get::<_, Uuid>(1).to_string(),
                "source_account_id": r.get::<_, String>(2),
                "file_key": r.get::<_, String>(3),
                "mime_type": r.get::<_, Option<String>>(4),
                "width": r.get::<_, Option<i32>>(5),
                "height": r.get::<_, Option<i32>>(6),
                "duration_ms": r.get::<_, Option<i64>>(7),
            })).collect();
            HttpResponse::Ok().json(items)
        }
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

pub async fn get_metadata_entry(
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
        "SELECT id, job_id, source_account_id, file_key, mime_type, width, height, duration_ms FROM np_fileproc_metadata WHERE id=$1 AND source_account_id=$2",
        &[&id, &sa],
    ).await {
        Ok(Some(r)) => HttpResponse::Ok().json(serde_json::json!({
            "id": r.get::<_, Uuid>(0).to_string(),
            "job_id": r.get::<_, Uuid>(1).to_string(),
            "source_account_id": r.get::<_, String>(2),
            "file_key": r.get::<_, String>(3),
            "mime_type": r.get::<_, Option<String>>(4),
            "width": r.get::<_, Option<i32>>(5),
            "height": r.get::<_, Option<i32>>(6),
            "duration_ms": r.get::<_, Option<i64>>(7),
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
        "SELECT COUNT(*) FILTER (WHERE status='pending') as pending, COUNT(*) FILTER (WHERE status='processing') as processing, COUNT(*) FILTER (WHERE status='completed') as completed FROM np_fileproc_jobs WHERE source_account_id=$1",
        &[&sa],
    ).await {
        Ok(r) => HttpResponse::Ok().json(serde_json::json!({
            "jobs": {
                "pending": r.get::<_, i64>(0),
                "processing": r.get::<_, i64>(1),
                "completed": r.get::<_, i64>(2),
            }
        })),
        Err(e) => HttpResponse::InternalServerError().json(serde_json::json!({"error": e.to_string()})),
    }
}

// ---------------------------------------------------------------------------
// Purpose: verify sa() — the multi-app tenant-isolation helper every handler
// in this file relies on to scope DB reads/writes to the caller's
// source_account_id (ASI/PPI Multi-Tenant Convention Wall). A regression
// here (e.g. wrong header name or wrong fallback) silently leaks or
// misattributes data across tenants, so this is real, load-bearing logic —
// not a wrapper. Uses actix_web::test to build a real HttpRequest without a
// live server/DB.
// ---------------------------------------------------------------------------
#[cfg(test)]
mod tests {
    use super::*;
    use actix_web::test::TestRequest;

    #[test]
    fn sa_defaults_to_primary_when_header_missing() {
        let req = TestRequest::default().to_http_request();
        assert_eq!(sa(&req), "primary");
    }

    #[test]
    fn sa_reads_source_account_header() {
        let req = TestRequest::default()
            .insert_header(("X-Source-Account-ID", "acct-42"))
            .to_http_request();
        assert_eq!(sa(&req), "acct-42");
    }

    #[test]
    fn sa_falls_back_on_non_utf8_header() {
        // An invalid UTF-8 header value should fail `to_str()` and fall back
        // to "primary" rather than panicking.
        let req = TestRequest::default()
            .insert_header(("X-Source-Account-ID", vec![0xff, 0xfe]))
            .to_http_request();
        assert_eq!(sa(&req), "primary");
    }
}
