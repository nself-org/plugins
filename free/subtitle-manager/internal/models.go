package internal

import (
	"encoding/json"
	"time"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config holds all subtitle-manager environment configuration.
type Config struct {
	DatabaseURL      string
	Port             int
	OpenSubtitlesKey string
	StoragePath      string
	LogLevel         string
	AlassPath        string
	FfsubsyncPath    string
}

// ---------------------------------------------------------------------------
// Subtitle record (np_subtmgr_subtitles)
// ---------------------------------------------------------------------------

// SubtitleRecord maps to a row in np_subtmgr_subtitles.
type SubtitleRecord struct {
	ID              string   `json:"id"`
	SourceAccountID string   `json:"source_account_id"`
	MediaID         string   `json:"media_id"`
	MediaType       string   `json:"media_type"`
	Language        string   `json:"language"`
	FilePath        string   `json:"file_path"`
	Source          string   `json:"source"`
	SyncScore       *float64 `json:"sync_score,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UpsertSubtitleInput is the input for creating or updating a subtitle record.
type UpsertSubtitleInput struct {
	SourceAccountID string   `json:"source_account_id,omitempty"`
	MediaID         string   `json:"media_id"`
	MediaType       string   `json:"media_type"`
	Language        string   `json:"language"`
	FilePath        string   `json:"file_path"`
	Source          string   `json:"source"`
	SyncScore       *float64 `json:"sync_score,omitempty"`
}

// ---------------------------------------------------------------------------
// Download record (np_subtmgr_downloads)
// ---------------------------------------------------------------------------

// DownloadRecord maps to a row in np_subtmgr_downloads.
type DownloadRecord struct {
	ID                   string           `json:"id"`
	SourceAccountID      string           `json:"source_account_id"`
	SubtitleID           *string          `json:"subtitle_id,omitempty"`
	MediaID              string           `json:"media_id"`
	MediaType            string           `json:"media_type"`
	MediaTitle           *string          `json:"media_title,omitempty"`
	Language             string           `json:"language"`
	FilePath             string           `json:"file_path"`
	FileSizeBytes        *int64           `json:"file_size_bytes,omitempty"`
	OpensubtitlesFileID  *int             `json:"opensubtitles_file_id,omitempty"`
	FileHash             *string          `json:"file_hash,omitempty"`
	SyncScore            *float64         `json:"sync_score,omitempty"`
	Source               string           `json:"source"`
	QCStatus             *string          `json:"qc_status,omitempty"`
	QCDetails            *json.RawMessage `json:"qc_details,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

// InsertDownloadInput is the input for inserting a new download record.
type InsertDownloadInput struct {
	SourceAccountID     string   `json:"source_account_id,omitempty"`
	SubtitleID          *string  `json:"subtitle_id,omitempty"`
	MediaID             string   `json:"media_id"`
	MediaType           string   `json:"media_type"`
	MediaTitle          string   `json:"media_title,omitempty"`
	Language            string   `json:"language"`
	FilePath            string   `json:"file_path"`
	FileSizeBytes       int64    `json:"file_size_bytes,omitempty"`
	OpensubtitlesFileID int      `json:"opensubtitles_file_id,omitempty"`
	FileHash            string   `json:"file_hash,omitempty"`
	SyncScore           *float64 `json:"sync_score,omitempty"`
	Source              string   `json:"source"`
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// SubtitleStats holds aggregated statistics.
type SubtitleStats struct {
	TotalSubtitles int             `json:"total_subtitles"`
	TotalDownloads int             `json:"total_downloads"`
	Languages      []LanguageCount `json:"languages"`
	Sources        []SourceCount   `json:"sources"`
}

// LanguageCount is a language with its download count.
type LanguageCount struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

// SourceCount is a source with its download count.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// ---------------------------------------------------------------------------
// QC types
// ---------------------------------------------------------------------------

// QCResult holds the full result of a quality check.
type QCResult struct {
	Status          string    `json:"status"`
	Checks          []QCCheck `json:"checks"`
	Issues          []QCIssue `json:"issues"`
	CueCount        int       `json:"cue_count"`
	TotalDurationMs int64     `json:"total_duration_ms"`
}

// QCCheck is a single named check result.
type QCCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}

// QCIssue is a single issue found during QC.
type QCIssue struct {
	Severity string `json:"severity"`
	Check    string `json:"check"`
	CueIndex *int   `json:"cue_index,omitempty"`
	Message  string `json:"message"`
}

// QCResultRecord maps to a row in np_subtmgr_qc_results.
type QCResultRecord struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	DownloadID      string    `json:"download_id"`
	Status          string    `json:"status"`
	Checks          []QCCheck `json:"checks"`
	Issues          []QCIssue `json:"issues"`
	CueCount        int       `json:"cue_count"`
	TotalDurationMs int64     `json:"total_duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

// InsertQCResultInput is the input for inserting a QC result.
type InsertQCResultInput struct {
	SourceAccountID string    `json:"source_account_id,omitempty"`
	DownloadID      string    `json:"download_id"`
	Status          string    `json:"status"`
	Checks          []QCCheck `json:"checks"`
	Issues          []QCIssue `json:"issues"`
	CueCount        int       `json:"cue_count"`
	TotalDurationMs int64     `json:"total_duration_ms"`
}

// QualityCheckDetails holds summary details stored on download record.
type QualityCheckDetails struct {
	CueCount   int `json:"cueCount"`
	IssueCount int `json:"issueCount"`
}

// ---------------------------------------------------------------------------
// Sync types
// ---------------------------------------------------------------------------

// SyncResult holds the output of the subtitle sync pipeline.
type SyncResult struct {
	OriginalPath    string             `json:"original_path"`
	SyncedPath      string             `json:"synced_path"`
	Confidence      float64            `json:"confidence"`
	OffsetMs        float64            `json:"offset_ms"`
	Method          string             `json:"method"`
	AlassResult     *AlassSyncResult   `json:"alass_result,omitempty"`
	FfsubsyncResult *FfsubsyncResult   `json:"ffsubsync_result,omitempty"`
}

// AlassSyncResult holds the output from alass.
type AlassSyncResult struct {
	Confidence        float64 `json:"confidence"`
	OffsetMs          float64 `json:"offset_ms"`
	FramerateAdjusted bool    `json:"framerate_adjusted"`
}

// FfsubsyncResult holds the output from ffsubsync.
type FfsubsyncResult struct {
	Confidence float64 `json:"confidence"`
	OffsetMs   float64 `json:"offset_ms"`
}

// SyncOptions controls which sync tools to use.
type SyncOptions struct {
	AlassOnly    bool
	FfsubsyncOnly bool
}

// ---------------------------------------------------------------------------
// Subtitle cue (parsing and QC)
// ---------------------------------------------------------------------------

// SubtitleCue represents a single subtitle cue for parsing and QC.
type SubtitleCue struct {
	Index   int    `json:"index"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// ---------------------------------------------------------------------------
// API request/response types
// ---------------------------------------------------------------------------

// SearchRequest is the body for POST /v1/search.
type SearchRequest struct {
	Query     string   `json:"query"`
	Languages []string `json:"languages,omitempty"`
}

// HashSearchRequest is the body for POST /v1/search/hash.
type HashSearchRequest struct {
	MovieHash     string   `json:"moviehash"`
	MovieByteSize int64    `json:"moviebytesize"`
	Languages     []string `json:"languages,omitempty"`
}

// DownloadRequest is the body for POST /v1/download.
type DownloadRequest struct {
	FileID    int    `json:"file_id"`
	MediaID   string `json:"media_id"`
	MediaType string `json:"media_type,omitempty"`
	MediaTitle string `json:"media_title,omitempty"`
	Language  string `json:"language,omitempty"`
	RunQC     bool   `json:"run_qc,omitempty"`
}

// SyncRequest is the body for POST /v1/sync.
type SyncRequest struct {
	VideoPath    string `json:"video_path"`
	SubtitlePath string `json:"subtitle_path"`
	Language     string `json:"language,omitempty"`
}

// QCRequest is the body for POST /v1/qc.
type QCRequest struct {
	SubtitlePath   string `json:"subtitle_path"`
	VideoDurationMs *int64 `json:"video_duration_ms,omitempty"`
	DownloadID     string `json:"download_id,omitempty"`
}

// NormalizeRequest is the body for POST /v1/normalize.
type NormalizeRequest struct {
	InputPath    string `json:"input_path"`
	OutputFormat string `json:"output_format,omitempty"`
}

// FetchBestRequest is the body for POST /v1/fetch-best.
type FetchBestRequest struct {
	VideoPath       string   `json:"video_path"`
	Languages       []string `json:"languages"`
	MaxAlternatives int      `json:"max_alternatives,omitempty"`
	MediaID         string   `json:"media_id,omitempty"`
	MediaType       string   `json:"media_type,omitempty"`
	MediaTitle      string   `json:"media_title,omitempty"`
}

// FetchBestLanguageResult is the per-language result from fetch-best.
type FetchBestLanguageResult struct {
	Language    string  `json:"language"`
	Path        *string `json:"path"`
	Format      string  `json:"format"`
	SyncQuality string  `json:"sync_quality"`
	SyncWarning bool    `json:"sync_warning"`
	OffsetMs    float64 `json:"offset_ms"`
	ToolUsed    string  `json:"tool_used"`
}
