package internal

import (
	"encoding/json"
	"time"
)

// RawJSON holds arbitrary JSON for fields like genres, credits, etc.
type RawJSON = json.RawMessage

// Request/response types

type SearchRequest struct {
	Query           string `json:"query"`
	MediaType       string `json:"media_type"`
	Year            *int   `json:"year,omitempty"`
	Language        string `json:"language,omitempty"`
	SourceAccountID string `json:"source_account_id,omitempty"`
}

type SearchResult struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	PosterPath   string  `json:"poster_path,omitempty"`
	BackdropPath string  `json:"backdrop_path,omitempty"`
	VoteAverage  float64 `json:"vote_average"`
	Popularity   float64 `json:"popularity"`
}

type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	TotalPages int            `json:"total_pages"`
	TotalCount int            `json:"total_count"`
}

type MatchRequest struct {
	MediaID         string `json:"media_id"`
	Filename        string `json:"filename,omitempty"`
	Title           string `json:"title,omitempty"`
	Year            *int   `json:"year,omitempty"`
	MediaType       string `json:"media_type,omitempty"`
	SourceAccountID string `json:"source_account_id,omitempty"`
}

type MatchResponse struct {
	QueueID      string         `json:"queue_id"`
	Status       string         `json:"status"`
	Confidence   float64        `json:"confidence"`
	BestMatch    *SearchResult  `json:"best_match,omitempty"`
	Candidates   []SearchResult `json:"candidates,omitempty"`
	AutoAccepted bool           `json:"auto_accepted"`
}

type BatchMatchRequest struct {
	Items           []MatchRequest `json:"items"`
	SourceAccountID string         `json:"source_account_id,omitempty"`
}

type BatchMatchResponse struct {
	Results []MatchResponse `json:"results"`
}

type ConfirmRequest struct {
	QueueID         string `json:"queue_id"`
	TMDBId          int    `json:"tmdb_id"`
	MediaType       string `json:"media_type"`
	ReviewedBy      string `json:"reviewed_by,omitempty"`
	SourceAccountID string `json:"source_account_id,omitempty"`
}

type RefreshRequest struct {
	TMDBID          int    `json:"tmdb_id"`
	MediaType       string `json:"media_type"`
	SourceAccountID string `json:"source_account_id,omitempty"`
}

type SyncRequest struct {
	SourceAccountID string `json:"source_account_id,omitempty"`
}

type StatusResponse struct {
	TotalMovies    int        `json:"total_movies"`
	TotalTVShows   int        `json:"total_tv_shows"`
	PendingMatches int        `json:"pending_matches"`
	TotalGenres    int        `json:"total_genres"`
	LastSyncedAt   *time.Time `json:"last_synced_at,omitempty"`
}

// TMDB API response types

type TMDBSearchResponse struct {
	Page         int              `json:"page"`
	Results      []TMDBSearchItem `json:"results"`
	TotalPages   int              `json:"total_pages"`
	TotalResults int              `json:"total_results"`
}

type TMDBSearchItem struct {
	ID               int     `json:"id"`
	MediaType        string  `json:"media_type"`
	Title            string  `json:"title,omitempty"`
	Name             string  `json:"name,omitempty"`
	Overview         string  `json:"overview"`
	ReleaseDate      string  `json:"release_date,omitempty"`
	FirstAirDate     string  `json:"first_air_date,omitempty"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	VoteAverage      float64 `json:"vote_average"`
	Popularity       float64 `json:"popularity"`
	OriginalLanguage string  `json:"original_language"`
}

type TMDBMovieDetails struct {
	ID                  int             `json:"id"`
	ImdbID              string          `json:"imdb_id"`
	Title               string          `json:"title"`
	OriginalTitle       string          `json:"original_title"`
	Overview            string          `json:"overview"`
	Tagline             string          `json:"tagline"`
	ReleaseDate         string          `json:"release_date"`
	Runtime             int             `json:"runtime"`
	Status              string          `json:"status"`
	PosterPath          string          `json:"poster_path"`
	BackdropPath        string          `json:"backdrop_path"`
	Budget              int64           `json:"budget"`
	Revenue             int64           `json:"revenue"`
	VoteAverage         float64         `json:"vote_average"`
	VoteCount           int             `json:"vote_count"`
	Popularity          float64         `json:"popularity"`
	OriginalLanguage    string          `json:"original_language"`
	Genres              []TMDBGenre     `json:"genres"`
	ProductionCompanies json.RawMessage `json:"production_companies"`
	ProductionCountries json.RawMessage `json:"production_countries"`
	SpokenLanguages     json.RawMessage `json:"spoken_languages"`
	Credits             json.RawMessage `json:"credits"`
	Keywords            json.RawMessage `json:"keywords"`
}

type TMDBTVDetails struct {
	ID               int             `json:"id"`
	Name             string          `json:"name"`
	OriginalName     string          `json:"original_name"`
	Overview         string          `json:"overview"`
	FirstAirDate     string          `json:"first_air_date"`
	LastAirDate      string          `json:"last_air_date"`
	Status           string          `json:"status"`
	Type             string          `json:"type"`
	NumberOfSeasons  int             `json:"number_of_seasons"`
	NumberOfEpisodes int             `json:"number_of_episodes"`
	EpisodeRunTime   []int           `json:"episode_run_time"`
	PosterPath       string          `json:"poster_path"`
	BackdropPath     string          `json:"backdrop_path"`
	VoteAverage      float64         `json:"vote_average"`
	VoteCount        int             `json:"vote_count"`
	Popularity       float64         `json:"popularity"`
	OriginalLanguage string          `json:"original_language"`
	Genres           []TMDBGenre     `json:"genres"`
	Networks         json.RawMessage `json:"networks"`
	CreatedBy        json.RawMessage `json:"created_by"`
	Credits          json.RawMessage `json:"credits"`
	ExternalIDs      struct {
		ImdbID string `json:"imdb_id"`
	} `json:"external_ids"`
}

type TMDBGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TMDBGenreList struct {
	Genres []TMDBGenre `json:"genres"`
}

type TMDBConfiguration struct {
	Images struct {
		BaseURL       string   `json:"base_url"`
		SecureBaseURL string   `json:"secure_base_url"`
		PosterSizes   []string `json:"poster_sizes"`
		BackdropSizes []string `json:"backdrop_sizes"`
	} `json:"images"`
}

type Movie struct {
	ID                   int             `json:"id"`
	SourceAccountID      string          `json:"source_account_id"`
	ImdbID               *string         `json:"imdb_id"`
	Title                string          `json:"title"`
	OriginalTitle        *string         `json:"original_title"`
	Overview             *string         `json:"overview"`
	Tagline              *string         `json:"tagline"`
	ReleaseDate          *string         `json:"release_date"`
	Runtime              *int            `json:"runtime"`
	Status               *string         `json:"status"`
	PosterPath           *string         `json:"poster_path"`
	BackdropPath         *string         `json:"backdrop_path"`
	Budget               *int64          `json:"budget"`
	Revenue              *int64          `json:"revenue"`
	VoteAverage          *float64        `json:"vote_average"`
	VoteCount            *int            `json:"vote_count"`
	Popularity           *float64        `json:"popularity"`
	OriginalLanguage     *string         `json:"original_language"`
	Genres               []byte          `json:"genres"`
	ProductionCompanies  []byte          `json:"production_companies"`
	ProductionCountries  []byte          `json:"production_countries"`
	SpokenLanguages      []byte          `json:"spoken_languages"`
	Credits              []byte          `json:"credits"`
	Keywords             []byte          `json:"keywords"`
	ContentRating        *string         `json:"content_rating"`
	SyncedAt             time.Time       `json:"synced_at"`
}

type TVShow struct {
	ID                int       `json:"id"`
	SourceAccountID   string    `json:"source_account_id"`
	ImdbID            *string   `json:"imdb_id"`
	Name              string    `json:"name"`
	OriginalName      *string   `json:"original_name"`
	Overview          *string   `json:"overview"`
	FirstAirDate      *string   `json:"first_air_date"`
	LastAirDate       *string   `json:"last_air_date"`
	Status            *string   `json:"status"`
	Type              *string   `json:"type"`
	NumberOfSeasons   *int      `json:"number_of_seasons"`
	NumberOfEpisodes  *int      `json:"number_of_episodes"`
	EpisodeRunTime    []byte    `json:"episode_run_time"`
	PosterPath        *string   `json:"poster_path"`
	BackdropPath      *string   `json:"backdrop_path"`
	VoteAverage       *float64  `json:"vote_average"`
	VoteCount         *int      `json:"vote_count"`
	Popularity        *float64  `json:"popularity"`
	OriginalLanguage  *string   `json:"original_language"`
	Genres            []byte    `json:"genres"`
	Networks          []byte    `json:"networks"`
	CreatedBy         []byte    `json:"created_by"`
	Credits           []byte    `json:"credits"`
	ContentRating     *string   `json:"content_rating"`
	SyncedAt          time.Time `json:"synced_at"`
}

type Season struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ShowID          int       `json:"show_id"`
	SeasonNumber    int       `json:"season_number"`
	Name            *string   `json:"name"`
	Overview        *string   `json:"overview"`
	PosterPath      *string   `json:"poster_path"`
	AirDate         *string   `json:"air_date"`
	EpisodeCount    *int      `json:"episode_count"`
	SyncedAt        time.Time `json:"synced_at"`
}

type Episode struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	ShowID          int       `json:"show_id"`
	SeasonNumber    int       `json:"season_number"`
	EpisodeNumber   int       `json:"episode_number"`
	Name            *string   `json:"name"`
	Overview        *string   `json:"overview"`
	StillPath       *string   `json:"still_path"`
	AirDate         *string   `json:"air_date"`
	Runtime         *int      `json:"runtime"`
	VoteAverage     *float64  `json:"vote_average"`
	Crew            []byte    `json:"crew"`
	GuestStars      []byte    `json:"guest_stars"`
	SyncedAt        time.Time `json:"synced_at"`
}

type Genre struct {
	ID              int    `json:"id"`
	SourceAccountID string `json:"source_account_id"`
	Name            string `json:"name"`
	MediaType       string `json:"media_type"`
}

type MatchQueue struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	MediaID         string     `json:"media_id"`
	Filename        *string    `json:"filename"`
	ParsedTitle     *string    `json:"parsed_title"`
	ParsedYear      *int       `json:"parsed_year"`
	ParsedType      *string    `json:"parsed_type"`
	MatchResults    []byte     `json:"match_results"`
	BestMatchID     *int       `json:"best_match_id"`
	BestMatchType   *string    `json:"best_match_type"`
	Confidence      *float64   `json:"confidence"`
	Status          string     `json:"status"`
	ReviewedBy      *string    `json:"reviewed_by"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
	AutoAccepted    bool       `json:"auto_accepted"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type WebhookEvent struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	EventType       string     `json:"event_type"`
	Payload         []byte     `json:"payload"`
	Processed       bool       `json:"processed"`
	ProcessedAt     *time.Time `json:"processed_at"`
	Error           *string    `json:"error"`
	RetryCount      int        `json:"retry_count"`
	CreatedAt       time.Time  `json:"created_at"`
}
