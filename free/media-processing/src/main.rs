mod config;
mod db;
mod handlers;
mod models;

use actix_web::{middleware, web, App, HttpServer};
use handlers::JobPermits;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{Mutex, Semaphore};

#[actix_web::main]
async fn main() -> std::io::Result<()> {
    env_logger::init_from_env(env_logger::Env::default().default_filter_or("info"));
    dotenvy::dotenv().ok();

    let cfg = config::Config::from_env().expect("Failed to load config");
    let port = cfg.port;
    let pool = Arc::new(
        db::create_pool(&cfg.database_url)
            .await
            .expect("Failed to create DB pool"),
    );

    let semaphore = Arc::new(Semaphore::new(cfg.max_concurrent_jobs));
    let job_permits = Arc::new(JobPermits(Mutex::new(HashMap::new())));
    log::info!(
        "media-processing starting on port {} (max_concurrent_jobs={})",
        port,
        cfg.max_concurrent_jobs
    );

    HttpServer::new(move || {
        App::new()
            .app_data(web::Data::new(pool.clone()))
            .app_data(web::Data::new(cfg.clone()))
            .app_data(web::Data::new(semaphore.clone()))
            .app_data(web::Data::new(job_permits.clone()))
            .wrap(middleware::Logger::default())
            .route("/health", web::get().to(handlers::health))
            .route("/ready", web::get().to(handlers::ready))
            // Encoding profiles
            .route("/api/v1/profiles", web::get().to(handlers::list_profiles))
            .route("/api/v1/profiles", web::post().to(handlers::create_profile))
            .route(
                "/api/v1/profiles/{id}",
                web::get().to(handlers::get_profile),
            )
            .route(
                "/api/v1/profiles/{id}",
                web::put().to(handlers::update_profile),
            )
            .route(
                "/api/v1/profiles/{id}",
                web::delete().to(handlers::delete_profile),
            )
            // Jobs
            .route("/api/v1/jobs", web::get().to(handlers::list_jobs))
            .route("/api/v1/jobs", web::post().to(handlers::create_job))
            .route("/api/v1/jobs/{id}", web::get().to(handlers::get_job))
            .route("/api/v1/jobs/{id}", web::delete().to(handlers::cancel_job))
            // Job outputs
            .route(
                "/api/v1/jobs/{id}/outputs",
                web::get().to(handlers::list_job_outputs),
            )
            // Uploads
            .route("/api/v1/uploads", web::post().to(handlers::create_upload))
            .route("/api/v1/uploads/{id}", web::get().to(handlers::get_upload))
            // Stats
            .route("/api/v1/stats", web::get().to(handlers::get_stats))
    })
    .bind(format!("0.0.0.0:{}", port))?
    .run()
    .await
}
