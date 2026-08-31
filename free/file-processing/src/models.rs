#![allow(dead_code)]
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

#[derive(Debug, Serialize, Deserialize)]
pub struct ProcessingJob {
    pub id: Uuid,
    pub source_account_id: String,
    pub file_key: String,
    pub file_url: String,
    pub content_type: Option<String>,
    pub file_size: Option<i64>,
    pub status: String,
    pub error_message: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Deserialize)]
pub struct CreateJobInput {
    pub file_key: String,
    pub file_url: String,
    pub content_type: Option<String>,
    pub file_size: Option<i64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Thumbnail {
    pub id: Uuid,
    pub job_id: Uuid,
    pub source_account_id: String,
    pub size: i32,
    pub url: String,
    pub width: Option<i32>,
    pub height: Option<i32>,
    pub file_size: Option<i64>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FileMetadata {
    pub id: Uuid,
    pub job_id: Uuid,
    pub source_account_id: String,
    pub file_key: String,
    pub mime_type: Option<String>,
    pub width: Option<i32>,
    pub height: Option<i32>,
    pub duration_ms: Option<i64>,
    pub metadata: Option<serde_json::Value>,
    pub created_at: DateTime<Utc>,
}

// ---------------------------------------------------------------------------
// Purpose: verify the wire-format (de)serialization these API response/
// request types actually rely on in production — CreateJobInput is parsed
// straight from client JSON bodies (handlers::create_job) and the others are
// serialized back out as API responses, so a silent field-rename or
// optional/required mismatch here is a real production bug, not busywork.
// ---------------------------------------------------------------------------
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn create_job_input_deserializes_required_fields_only() {
        let json = r#"{"file_key":"k1","file_url":"https://x/y"}"#;
        let input: CreateJobInput = serde_json::from_str(json).unwrap();
        assert_eq!(input.file_key, "k1");
        assert_eq!(input.file_url, "https://x/y");
        assert_eq!(input.content_type, None);
        assert_eq!(input.file_size, None);
    }

    #[test]
    fn create_job_input_deserializes_optional_fields() {
        let json = r#"{"file_key":"k1","file_url":"https://x/y","content_type":"image/png","file_size":1024}"#;
        let input: CreateJobInput = serde_json::from_str(json).unwrap();
        assert_eq!(input.content_type.as_deref(), Some("image/png"));
        assert_eq!(input.file_size, Some(1024));
    }

    #[test]
    fn create_job_input_rejects_missing_required_field() {
        let json = r#"{"file_key":"k1"}"#;
        let result: Result<CreateJobInput, _> = serde_json::from_str(json);
        assert!(result.is_err());
    }

    #[test]
    fn processing_job_round_trips_through_json() {
        let job = ProcessingJob {
            id: Uuid::new_v4(),
            source_account_id: "primary".to_string(),
            file_key: "uploads/a.png".to_string(),
            file_url: "https://cdn/uploads/a.png".to_string(),
            content_type: Some("image/png".to_string()),
            file_size: Some(2048),
            status: "pending".to_string(),
            error_message: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        };

        let serialized = serde_json::to_string(&job).unwrap();
        let deserialized: ProcessingJob = serde_json::from_str(&serialized).unwrap();

        assert_eq!(deserialized.id, job.id);
        assert_eq!(deserialized.file_key, job.file_key);
        assert_eq!(deserialized.status, job.status);
        assert_eq!(deserialized.content_type, job.content_type);
        assert_eq!(deserialized.error_message, None);
    }

    #[test]
    fn thumbnail_serializes_with_expected_fields() {
        let thumb = Thumbnail {
            id: Uuid::new_v4(),
            job_id: Uuid::new_v4(),
            source_account_id: "primary".to_string(),
            size: 400,
            url: "https://cdn/thumbs/400.png".to_string(),
            width: Some(400),
            height: Some(300),
            file_size: Some(512),
            created_at: Utc::now(),
        };

        let value = serde_json::to_value(&thumb).unwrap();
        assert_eq!(value["size"], 400);
        assert_eq!(value["width"], 400);
        assert_eq!(value["url"], "https://cdn/thumbs/400.png");
    }

    #[test]
    fn file_metadata_allows_null_optional_fields() {
        let json = r#"{
            "id":"00000000-0000-0000-0000-000000000001",
            "job_id":"00000000-0000-0000-0000-000000000002",
            "source_account_id":"primary",
            "file_key":"a.mp4",
            "mime_type":null,
            "width":null,
            "height":null,
            "duration_ms":null,
            "metadata":null,
            "created_at":"2026-01-01T00:00:00Z"
        }"#;
        let meta: FileMetadata = serde_json::from_str(json).unwrap();
        assert_eq!(meta.file_key, "a.mp4");
        assert!(meta.mime_type.is_none());
        assert!(meta.duration_ms.is_none());
    }
}
