#[derive(Clone, Debug)]
pub struct Config {
    pub port: u16,
    pub database_url: String,
    #[allow(dead_code)]
    pub storage_bucket: String,
    #[allow(dead_code)]
    pub storage_provider: String,
    #[allow(dead_code)]
    pub max_file_size: u64,
    #[allow(dead_code)]
    pub thumbnail_sizes: Vec<u32>,
}

impl Config {
    pub fn from_env() -> Result<Self, String> {
        let thumbnail_sizes = std::env::var("FILE_THUMBNAIL_SIZES")
            .unwrap_or_else(|_| "100,400,1200".to_string())
            .split(',')
            .filter_map(|s| s.trim().parse().ok())
            .collect();

        Ok(Self {
            port: std::env::var("PORT")
                .unwrap_or_else(|_| "3089".to_string())
                .parse()
                .map_err(|e| format!("Invalid port: {e}"))?,
            database_url: std::env::var("DATABASE_URL")
                .map_err(|_| "DATABASE_URL required".to_string())?,
            storage_bucket: std::env::var("FILE_STORAGE_BUCKET")
                .unwrap_or_else(|_| "files".to_string()),
            storage_provider: std::env::var("FILE_STORAGE_PROVIDER")
                .unwrap_or_else(|_| "minio".to_string()),
            max_file_size: std::env::var("FILE_MAX_SIZE")
                .unwrap_or_else(|_| "104857600".to_string())
                .parse()
                .unwrap_or(104857600),
            thumbnail_sizes,
        })
    }
}

// ---------------------------------------------------------------------------
// Purpose: verify Config::from_env's parsing/defaulting logic (ports, sizes,
// thumbnail-size CSV parsing) without requiring a live process environment
// per-test-run ordering guarantee — std::env is process-global, so each test
// saves and restores only the vars it touches and runs serially via a mutex
// to avoid cross-test env races (tests in the same binary run in threads).
// ---------------------------------------------------------------------------
#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Serializes env-mutating tests since `std::env::set_var` is process-wide
    // and cargo test runs tests in this file concurrently by default.
    static ENV_LOCK: Mutex<()> = Mutex::new(());

    fn clear_config_env() {
        for key in [
            "PORT",
            "DATABASE_URL",
            "FILE_STORAGE_BUCKET",
            "FILE_STORAGE_PROVIDER",
            "FILE_MAX_SIZE",
            "FILE_THUMBNAIL_SIZES",
        ] {
            std::env::remove_var(key);
        }
    }

    #[test]
    fn from_env_requires_database_url() {
        let _guard = ENV_LOCK.lock().unwrap();
        clear_config_env();
        let result = Config::from_env();
        assert!(result.is_err());
        assert_eq!(result.unwrap_err(), "DATABASE_URL required");
    }

    #[test]
    fn from_env_applies_defaults() {
        let _guard = ENV_LOCK.lock().unwrap();
        clear_config_env();
        std::env::set_var("DATABASE_URL", "postgres://localhost/test");

        let cfg = Config::from_env().expect("config should build with only DATABASE_URL set");
        assert_eq!(cfg.port, 3089);
        assert_eq!(cfg.storage_bucket, "files");
        assert_eq!(cfg.storage_provider, "minio");
        assert_eq!(cfg.max_file_size, 104_857_600);
        assert_eq!(cfg.thumbnail_sizes, vec![100, 400, 1200]);

        clear_config_env();
    }

    #[test]
    fn from_env_parses_overrides() {
        let _guard = ENV_LOCK.lock().unwrap();
        clear_config_env();
        std::env::set_var("DATABASE_URL", "postgres://localhost/test");
        std::env::set_var("PORT", "9090");
        std::env::set_var("FILE_STORAGE_BUCKET", "custom-bucket");
        std::env::set_var("FILE_STORAGE_PROVIDER", "s3");
        std::env::set_var("FILE_MAX_SIZE", "2048");
        std::env::set_var("FILE_THUMBNAIL_SIZES", "50, 150,300");

        let cfg = Config::from_env().expect("config should build with overrides");
        assert_eq!(cfg.port, 9090);
        assert_eq!(cfg.storage_bucket, "custom-bucket");
        assert_eq!(cfg.storage_provider, "s3");
        assert_eq!(cfg.max_file_size, 2048);
        assert_eq!(cfg.thumbnail_sizes, vec![50, 150, 300]);

        clear_config_env();
    }

    #[test]
    fn from_env_rejects_invalid_port() {
        let _guard = ENV_LOCK.lock().unwrap();
        clear_config_env();
        std::env::set_var("DATABASE_URL", "postgres://localhost/test");
        std::env::set_var("PORT", "not-a-port");

        let result = Config::from_env();
        assert!(result.is_err());
        assert!(result.unwrap_err().starts_with("Invalid port:"));

        clear_config_env();
    }

    #[test]
    fn from_env_falls_back_on_invalid_max_size() {
        let _guard = ENV_LOCK.lock().unwrap();
        clear_config_env();
        std::env::set_var("DATABASE_URL", "postgres://localhost/test");
        std::env::set_var("FILE_MAX_SIZE", "not-a-number");

        let cfg = Config::from_env().expect("invalid max size falls back to default");
        assert_eq!(cfg.max_file_size, 104_857_600);

        clear_config_env();
    }
}
