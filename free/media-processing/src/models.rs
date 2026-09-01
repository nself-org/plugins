#![allow(dead_code)]

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct EncodingProfile {
    pub id: Uuid,
    pub source_account_id: String,
    pub name: String,
    pub description: Option<String>,
    pub is_default: bool,
    pub video_codec: Option<String>,
    pub audio_codec: Option<String>,
    pub container: Option<String>,
    pub bitrate_kbps: Option<i32>,
    pub width: Option<i32>,
    pub height: Option<i32>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize)]
pub struct CreateProfileInput {
    pub name: String,
    pub description: Option<String>,
    pub is_default: Option<bool>,
    pub video_codec: Option<String>,
    pub audio_codec: Option<String>,
    pub container: Option<String>,
    pub bitrate_kbps: Option<i32>,
    pub width: Option<i32>,
    pub height: Option<i32>,
}

#[derive(Debug, Deserialize)]
pub struct UpdateProfileInput {
    pub name: Option<String>,
    pub description: Option<String>,
    pub is_default: Option<bool>,
    pub video_codec: Option<String>,
    pub audio_codec: Option<String>,
}

#[derive(Debug, Serialize, Deserialize, Clone, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum JobStatus {
    Pending,
    Processing,
    Completed,
    Failed,
    Cancelled,
}

impl std::fmt::Display for JobStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let s = match self {
            JobStatus::Pending => "pending",
            JobStatus::Processing => "processing",
            JobStatus::Completed => "completed",
            JobStatus::Failed => "failed",
            JobStatus::Cancelled => "cancelled",
        };
        write!(f, "{s}")
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Job {
    pub id: Uuid,
    pub source_account_id: String,
    pub profile_id: Option<Uuid>,
    pub input_url: String,
    pub output_path: Option<String>,
    pub status: String,
    pub priority: i32,
    pub error_message: Option<String>,
    pub progress_pct: Option<f64>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize)]
pub struct CreateJobInput {
    pub profile_id: Option<Uuid>,
    pub input_url: String,
    pub output_path: Option<String>,
    pub priority: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct JobOutput {
    pub id: Uuid,
    pub job_id: Uuid,
    pub source_account_id: String,
    pub output_type: String,
    pub url: String,
    pub size_bytes: Option<i64>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Upload {
    pub id: Uuid,
    pub source_account_id: String,
    pub filename: String,
    pub file_url: String,
    pub file_size: Option<i64>,
    pub content_type: Option<String>,
    pub status: String,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize)]
pub struct CreateUploadInput {
    pub filename: String,
    pub file_url: String,
    pub file_size: Option<i64>,
    pub content_type: Option<String>,
}
