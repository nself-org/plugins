package internal

import "os"

type Config struct {
	DatabaseURL          string
	Port                 string
	TMDBApiKey           string
	TMDBApiReadToken     string
	OMDBApiKey           string
	TVDBApiKey           string
	MusicBrainzUserAgent string
	AutoAcceptThreshold  float64
	DefaultLanguage      string
	CacheTTLDays         int
	AppIDs               string
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3122"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}
	lang := os.Getenv("TMDB_DEFAULT_LANGUAGE")
	if lang == "" {
		lang = "en-US"
	}
	return Config{
		DatabaseURL:          dbURL,
		Port:                 port,
		TMDBApiKey:           os.Getenv("TMDB_API_KEY"),
		TMDBApiReadToken:     os.Getenv("TMDB_API_READ_ACCESS_TOKEN"),
		OMDBApiKey:           os.Getenv("OMDB_API_KEY"),
		TVDBApiKey:           os.Getenv("TVDB_API_KEY"),
		MusicBrainzUserAgent: os.Getenv("MUSICBRAINZ_USER_AGENT"),
		AutoAcceptThreshold:  0.85,
		DefaultLanguage:      lang,
		CacheTTLDays:         30,
		AppIDs:               os.Getenv("TMDB_APP_IDS"),
	}
}
