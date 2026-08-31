package internal

import "time"

type Podcast struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description,omitempty"`
	Author          string    `json:"author,omitempty"`
	ImageURL        string    `json:"image_url,omitempty"`
	Language        string    `json:"language"`
	IsPublic        bool      `json:"is_public"`
	EpisodeCount    int       `json:"episode_count"`
	SubscriberCount int       `json:"subscriber_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Episode struct {
	ID              string     `json:"id"`
	PodcastID       string     `json:"podcast_id"`
	SourceAccountID string     `json:"source_account_id"`
	Title           string     `json:"title"`
	Description     string     `json:"description,omitempty"`
	AudioURL        string     `json:"audio_url"`
	Duration        int        `json:"duration_seconds"`
	Season          int        `json:"season,omitempty"`
	Episode         int        `json:"episode_number,omitempty"`
	IsPublished     bool       `json:"is_published"`
	PlayCount       int        `json:"play_count"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Subscription struct {
	ID              string    `json:"id"`
	PodcastID       string    `json:"podcast_id"`
	SourceAccountID string    `json:"source_account_id"`
	UserID          string    `json:"user_id"`
	CreatedAt       time.Time `json:"created_at"`
}
