package internal

import "time"

// Document represents a row in docs_documents.
type Document struct {
	ID               string     `json:"id"`
	SourceAccountID  string     `json:"source_account_id"`
	OwnerID          string     `json:"owner_id"`
	Title            string     `json:"title"`
	Description      *string    `json:"description"`
	DocType          string     `json:"doc_type"`
	Category         *string    `json:"category"`
	Tags             []string   `json:"tags"`
	TemplateID       *string    `json:"template_id"`
	FileURL          *string    `json:"file_url"`
	FileSizeBytes    *int64     `json:"file_size_bytes"`
	MimeType         *string    `json:"mime_type"`
	Version          int        `json:"version"`
	Status           string     `json:"status"`
	GeneratedFrom    *string    `json:"generated_from"`
	Metadata         string     `json:"metadata"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Template represents a row in docs_templates.
type Template struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Name            string    `json:"name"`
	Description     *string   `json:"description"`
	DocType         string    `json:"doc_type"`
	OutputFormat    string    `json:"output_format"`
	TemplateEngine  string    `json:"template_engine"`
	TemplateContent string    `json:"template_content"`
	CSSContent      *string   `json:"css_content"`
	HeaderContent   *string   `json:"header_content"`
	FooterContent   *string   `json:"footer_content"`
	Variables       string    `json:"variables"`
	SampleData      string    `json:"sample_data"`
	IsDefault       bool      `json:"is_default"`
	Version         int       `json:"version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DocVersion represents a row in docs_versions.
type DocVersion struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	DocumentID       string    `json:"document_id"`
	Version          int       `json:"version"`
	FileURL          string    `json:"file_url"`
	FileSizeBytes    *int64    `json:"file_size_bytes"`
	ChangeSummary    *string   `json:"change_summary"`
	CreatedBy        *string   `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

// Share represents a row in docs_shares.
type Share struct {
	ID                 string     `json:"id"`
	SourceAccountID    string     `json:"source_account_id"`
	DocumentID         string     `json:"document_id"`
	SharedWithUserID   *string    `json:"shared_with_user_id"`
	SharedWithEmail    *string    `json:"shared_with_email"`
	ShareToken         *string    `json:"share_token"`
	Permission         string     `json:"permission"`
	ExpiresAt          *time.Time `json:"expires_at"`
	AccessedAt         *time.Time `json:"accessed_at"`
	CreatedAt          time.Time  `json:"created_at"`
}
