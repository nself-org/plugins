package internal

import "time"

// Token is the DB representation of a connected LinkedIn account.
type Token struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	AccessToken     string     `json:"-"` // never exposed in API responses
	ExpiresAt       time.Time  `json:"expires_at"`
	LinkedInID      string     `json:"linkedin_id"`
	Name            string     `json:"name"`
	Email           *string    `json:"email,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// StatusResponse is returned by GET /linkedin/status.
type StatusResponse struct {
	Connected  bool      `json:"connected"`
	Name       string    `json:"name,omitempty"`
	Email      *string   `json:"email,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

// PublishRequest is the JSON body for POST /linkedin/publish.
type PublishRequest struct {
	Text       string  `json:"text"`
	Visibility string  `json:"visibility,omitempty"` // PUBLIC | CONNECTIONS — defaults to PUBLIC
	ImageURL   *string `json:"image_url,omitempty"`
}

// PublishResponse is returned after a successful publish.
type PublishResponse struct {
	ID            string  `json:"id"`
	LinkedInPostID *string `json:"linkedin_post_id,omitempty"`
	Status        string  `json:"status"`
}

// PostRecord is a row from np_linkedin_posts returned to callers.
type PostRecord struct {
	ID             string     `json:"id"`
	LinkedInPostID *string    `json:"linkedin_post_id,omitempty"`
	Text           string     `json:"text"`
	ImageURL       *string    `json:"image_url,omitempty"`
	Visibility     string     `json:"visibility"`
	Status         string     `json:"status"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
	PublishedAt    time.Time  `json:"published_at"`
}

// ToolDescriptor describes the linkedin.publish_post tool for Claw.
type ToolDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}
