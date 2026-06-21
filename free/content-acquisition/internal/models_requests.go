package internal

import (
	"encoding/json"
)

type CreateSubscriptionRequest struct {
	ContentType      string  `json:"contentType"`
	ContentID        *string `json:"contentId,omitempty"`
	ContentName      string  `json:"contentName"`
	QualityProfileID *string `json:"qualityProfileId,omitempty"`
}

type UpdateSubscriptionRequest struct {
	ContentType      *string `json:"contentType,omitempty"`
	ContentID        *string `json:"contentId,omitempty"`
	ContentName      *string `json:"contentName,omitempty"`
	QualityProfileID *string `json:"qualityProfileId,omitempty"`
	Enabled          *bool   `json:"enabled,omitempty"`
	AutoUpgrade      *bool   `json:"autoUpgrade,omitempty"`
}

type CreateFeedRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	FeedType string `json:"feedType"`
}

type UpdateFeedRequest struct {
	Name                 *string `json:"name,omitempty"`
	URL                  *string `json:"url,omitempty"`
	FeedType             *string `json:"feedType,omitempty"`
	Enabled              *bool   `json:"enabled,omitempty"`
	CheckIntervalMinutes *int    `json:"checkIntervalMinutes,omitempty"`
}

type ValidateFeedRequest struct {
	URL string `json:"url"`
}

type AddToQueueRequest struct {
	ContentType string `json:"contentType"`
	ContentName string `json:"contentName"`
	Year        *int   `json:"year,omitempty"`
	Season      *int   `json:"season,omitempty"`
	Episode     *int   `json:"episode,omitempty"`
}

type CreateProfileRequest struct {
	Name               string   `json:"name"`
	PreferredQualities []string `json:"preferredQualities,omitempty"`
	MinSeeders         *int     `json:"minSeeders,omitempty"`
}

type CreateMovieRequest struct {
	Title          string  `json:"title"`
	TmdbID         *int    `json:"tmdbId,omitempty"`
	QualityProfile *string `json:"qualityProfile,omitempty"`
	AutoDownload   *bool   `json:"autoDownload,omitempty"`
	AutoUpgrade    *bool   `json:"autoUpgrade,omitempty"`
}

type UpdateMovieRequest struct {
	Title          *string `json:"title,omitempty"`
	TmdbID         *int    `json:"tmdbId,omitempty"`
	QualityProfile *string `json:"qualityProfile,omitempty"`
	AutoDownload   *bool   `json:"autoDownload,omitempty"`
	AutoUpgrade    *bool   `json:"autoUpgrade,omitempty"`
	Status         *string `json:"status,omitempty"`
}

type CreateDownloadRequest struct {
	ContentType    string  `json:"contentType"`
	Title          string  `json:"title"`
	MagnetURI      *string `json:"magnetUri,omitempty"`
	QualityProfile *string `json:"qualityProfile,omitempty"`
	ShowID         *string `json:"showId,omitempty"`
	SeasonNumber   *int    `json:"seasonNumber,omitempty"`
	EpisodeNumber  *int    `json:"episodeNumber,omitempty"`
	TmdbID         *int    `json:"tmdbId,omitempty"`
}

type CreateRuleRequest struct {
	Name       string          `json:"name"`
	Conditions json.RawMessage `json:"conditions"`
	Action     string          `json:"action"`
	Priority   *int            `json:"priority,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
}

type UpdateRuleRequest struct {
	Name       *string          `json:"name,omitempty"`
	Conditions *json.RawMessage `json:"conditions,omitempty"`
	Action     *string          `json:"action,omitempty"`
	Priority   *int             `json:"priority,omitempty"`
	Enabled    *bool            `json:"enabled,omitempty"`
}

type TestRuleRequest struct {
	Sample map[string]interface{} `json:"sample"`
}

type PipelineTriggerRequest struct {
	ContentTitle string  `json:"content_title"`
	ContentType  *string `json:"content_type,omitempty"`
	MagnetURL    *string `json:"magnet_url,omitempty"`
	TorrentURL   *string `json:"torrent_url,omitempty"`
}

type RSSPollRequest struct {
	URL      string                   `json:"url"`
	Criteria []map[string]interface{} `json:"criteria"`
	LastSeen *string                  `json:"lastSeen,omitempty"`
}

type RSSTestRequest struct {
	URL string `json:"url"`
}
