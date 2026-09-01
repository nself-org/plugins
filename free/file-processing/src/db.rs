use deadpool_postgres::{Config as PgConfig, Pool, Runtime};
use tokio_postgres::NoTls;

pub type DbPool = Pool;

pub async fn create_pool(database_url: &str) -> Result<DbPool, Box<dyn std::error::Error>> {
    let mut cfg = PgConfig::new();
    cfg.url = Some(database_url.to_string());
    let pool = cfg.create_pool(Some(Runtime::Tokio1), NoTls)?;
    let _client = pool.get().await?;
    Ok(pool)
}
