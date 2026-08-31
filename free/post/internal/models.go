package internal

import (
	"time"
)

// PostAccount is the full DB representation of a publishing account.
// Credentials are stored encrypted; this struct holds the decrypted form
// and is never serialised directly to callers.
type PostAccount struct {
	ID              string                 `json:"-"`
	SourceAccountID string                 `json:"-"`
	Platform        string                 `json:"platform"`
	Name            string                 `json:"name"`
	Endpoint        *string                `json:"endpoint,omitempty"`
	Credentials     map[string]interface{} `json:"-"`
	CreatedAt       time.Time              `json:"-"`
}

// PostAccountView is the safe response shape returned to callers.
// Credentials are intentionally omitted.
type PostAccountView struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Platform        string    `json:"platform"`
	Name            string    `json:"name"`
	Endpoint        *string   `json:"endpoint,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateAccountRequest is the JSON body for POST /post/accounts.
type CreateAccountRequest struct {
	SourceAccountID *string                `json:"source_account_id"`
	Platform        string                 `json:"platform"`
	Name            string                 `json:"name"`
	Endpoint        *string                `json:"endpoint,omitempty"`
	Credentials     map[string]interface{} `json:"credentials"`
}

// QueueEntry represents a row in np_post_queue.
type QueueEntry struct {
	ID              string                 `json:"id"`
	SourceAccountID string                 `json:"source_account_id"`
	Platform        string                 `json:"platform"`
	AccountID       string                 `json:"account_id"`
	Title           string                 `json:"title"`
	Content         string                 `json:"content"`
	Tags            []string               `json:"tags"`
	ScheduleAt      *time.Time             `json:"schedule_at,omitempty"`
	Status          string                 `json:"status"`
	Result          map[string]interface{} `json:"result,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
}

// PublishRequest is the JSON body for POST /post/publish.
type PublishRequest struct {
	AccountID  string     `json:"account_id"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	Tags       []string   `json:"tags,omitempty"`
	ScheduleAt *time.Time `json:"schedule_at,omitempty"`
}

// PublishResponse is returned after a publish or schedule operation.
type PublishResponse struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	URL    *string `json:"url,omitempty"`
}
