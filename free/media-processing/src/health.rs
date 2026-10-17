//! GET /health: liveness plus boot-migration progress (contract plugin.boot-migrations v1).
use actix_web::{web, HttpResponse};
use deadpool_postgres::Pool;
use std::sync::Arc;

/// 200 with the applied/expected counts; 503 when the ledger cannot be read.
pub async fn health(pool: web::Data<Arc<Pool>>) -> HttpResponse {
    match crate::migrate::status(&pool).await {
        Ok(p) => HttpResponse::Ok().json(serde_json::json!({"status": "ok", "migrations": p.to_json()})),
        Err(e) => {
            log::error!("health: {e}");
            HttpResponse::ServiceUnavailable()
                .json(serde_json::json!({"status": "error", "error": "migration status unavailable"}))
        }
    }
}
