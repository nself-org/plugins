package internal

import "time"

type Platform struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Name            string     `json:"name"`
	Abbreviation    *string    `json:"abbreviation,omitempty"`
	Slug            string     `json:"slug"`
	IgdbID          *int       `json:"igdb_id,omitempty"`
	Generation      *int       `json:"generation,omitempty"`
	Manufacturer    *string    `json:"manufacturer,omitempty"`
	PlatformFamily  *string    `json:"platform_family,omitempty"`
	Category        *string    `json:"category,omitempty"`
	ReleaseDate     *time.Time `json:"release_date,omitempty"`
	Summary         *string    `json:"summary,omitempty"`
	IsActive        bool       `json:"is_active"`
	SortOrder       int        `json:"sort_order"`
	Metadata        []byte     `json:"metadata"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Genre struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	IgdbID          *int       `json:"igdb_id,omitempty"`
	Description     *string    `json:"description,omitempty"`
	ParentID        *string    `json:"parent_id,omitempty"`
	IsActive        bool       `json:"is_active"`
	SortOrder       int        `json:"sort_order"`
	Metadata        []byte     `json:"metadata"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Game struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	Title           string     `json:"title"`
	Slug            string     `json:"slug"`
	PlatformID      *string    `json:"platform_id,omitempty"`
	GenreID         *string    `json:"genre_id,omitempty"`
	ReleaseDate     *time.Time `json:"release_date,omitempty"`
	Developer       *string    `json:"developer,omitempty"`
	Publisher       *string    `json:"publisher,omitempty"`
	Description     *string    `json:"description,omitempty"`
	IgdbID          *int       `json:"igdb_id,omitempty"`
	RomHashMd5      *string    `json:"rom_hash_md5,omitempty"`
	RomHashSha1     *string    `json:"rom_hash_sha1,omitempty"`
	RomHashSha256   *string    `json:"rom_hash_sha256,omitempty"`
	RomHashCrc32    *string    `json:"rom_hash_crc32,omitempty"`
	RomFilename     *string    `json:"rom_filename,omitempty"`
	RomSizeBytes    *int64     `json:"rom_size_bytes,omitempty"`
	Tier            *string    `json:"tier,omitempty"`
	Rating          *float64   `json:"rating,omitempty"`
	PlayersMin      int        `json:"players_min"`
	PlayersMax      int        `json:"players_max"`
	IsVerified      bool       `json:"is_verified"`
	Metadata        []byte     `json:"metadata"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type GameMetadata struct {
	ID                    string     `json:"id"`
	SourceAccountID       string     `json:"source_account_id"`
	GameID                string     `json:"game_id"`
	Source                string     `json:"source"`
	IgdbID                *int       `json:"igdb_id,omitempty"`
	IgdbURL               *string    `json:"igdb_url,omitempty"`
	Summary               *string    `json:"summary,omitempty"`
	Storyline             *string    `json:"storyline,omitempty"`
	TotalRating           *float64   `json:"total_rating,omitempty"`
	TotalRatingCount      *int       `json:"total_rating_count,omitempty"`
	AggregatedRating      *float64   `json:"aggregated_rating,omitempty"`
	AggregatedRatingCount *int       `json:"aggregated_rating_count,omitempty"`
	FirstReleaseDate      *time.Time `json:"first_release_date,omitempty"`
	Genres                []string   `json:"genres"`
	Themes                []string   `json:"themes"`
	Keywords              []string   `json:"keywords"`
	GameModes             []string   `json:"game_modes"`
	Franchises            []string   `json:"franchises"`
	AlternativeNames      []string   `json:"alternative_names"`
	Websites              []byte     `json:"websites"`
	AgeRatings            []byte     `json:"age_ratings"`
	InvolvedCompanies     []byte     `json:"involved_companies"`
	RawData               []byte     `json:"raw_data"`
	FetchedAt             time.Time  `json:"fetched_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type Artwork struct {
	ID              string    `json:"id"`
	SourceAccountID string    `json:"source_account_id"`
	GameID          string    `json:"game_id"`
	ArtworkType     string    `json:"artwork_type"`
	URL             *string   `json:"url,omitempty"`
	LocalPath       *string   `json:"local_path,omitempty"`
	Width           *int      `json:"width,omitempty"`
	Height          *int      `json:"height,omitempty"`
	MimeType        *string   `json:"mime_type,omitempty"`
	FileSizeBytes   *int64    `json:"file_size_bytes,omitempty"`
	Source          string    `json:"source"`
	IgdbImageID     *string   `json:"igdb_image_id,omitempty"`
	IsPrimary       bool      `json:"is_primary"`
	SortOrder       int       `json:"sort_order"`
	Metadata        []byte    `json:"metadata"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Stats struct {
	TotalGames      int            `json:"total_games"`
	VerifiedGames   int            `json:"verified_games"`
	TotalPlatforms  int            `json:"total_platforms"`
	TotalGenres     int            `json:"total_genres"`
	TotalArtwork    int            `json:"total_artwork"`
	TotalMetadata   int            `json:"total_metadata"`
	GamesWithIgdb   int            `json:"games_with_igdb"`
	GamesWithHashes int            `json:"games_with_hashes"`
	TierBreakdown   map[string]int `json:"tier_breakdown"`
}
