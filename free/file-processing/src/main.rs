mod config;
mod db;
mod handlers;
mod models;

use actix_web::{middleware, web, App, HttpServer};
use std::sync::Arc;

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

    log::info!("file-processing starting on port {}", port);

    HttpServer::new(move || {
        App::new()
            .app_data(web::Data::new(pool.clone()))
            .app_data(web::Data::new(cfg.clone()))
            .wrap(middleware::Logger::default())
            .route("/health", web::get().to(handlers::health))
            .route("/ready", web::get().to(handlers::ready))
            // Jobs
            .route("/api/v1/jobs", web::get().to(handlers::list_jobs))
            .route("/api/v1/jobs", web::post().to(handlers::create_job))
            .route("/api/v1/jobs/{id}", web::get().to(handlers::get_job))
            .route("/api/v1/jobs/{id}", web::delete().to(handlers::cancel_job))
            // Thumbnails
            .route(
                "/api/v1/thumbnails",
                web::get().to(handlers::list_thumbnails),
            )
            .route(
                "/api/v1/thumbnails/{id}",
                web::get().to(handlers::get_thumbnail),
            )
            // Metadata
            .route("/api/v1/metadata", web::get().to(handlers::list_metadata))
            .route(
                "/api/v1/metadata/{id}",
                web::get().to(handlers::get_metadata_entry),
            )
            // Stats
            .route("/api/v1/stats", web::get().to(handlers::get_stats))
    })
    .bind(format!("0.0.0.0:{}", port))?
    .run()
    .await
}
