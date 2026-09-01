#![allow(dead_code)]

#[derive(Clone)]
pub struct Config {
    pub port: u16,
    pub database_url: String,
    pub ffmpeg_path: String,
    pub ffprobe_path: String,
    pub output_base_path: String,
    pub max_concurrent_jobs: usize,
}

impl Config {
    pub fn from_env() -> Result<Self, String> {
        Ok(Self {
            port: std::env::var("PORT")
                .or_else(|_| std::env::var("MP_PLUGIN_PORT"))
                .unwrap_or_else(|_| "3088".to_string())
                .parse()
                .map_err(|e| format!("Invalid port: {e}"))?,
            database_url: std::env::var("DATABASE_URL")
                .map_err(|_| "DATABASE_URL required".to_string())?,
            ffmpeg_path: std::env::var("MP_FFMPEG_PATH").unwrap_or_else(|_| "ffmpeg".to_string()),
            ffprobe_path: std::env::var("MP_FFPROBE_PATH")
                .unwrap_or_else(|_| "ffprobe".to_string()),
            output_base_path: std::env::var("MP_OUTPUT_BASE_PATH")
                .unwrap_or_else(|_| "/data/media-processing".to_string()),
            max_concurrent_jobs: std::env::var("MP_MAX_CONCURRENT_JOBS")
                .unwrap_or_else(|_| "2".to_string())
                .parse()
                .unwrap_or(2),
        })
    }
}
