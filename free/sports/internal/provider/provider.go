package provider

import (
	"context"
	"time"
)

// Sport is a well-known sport identifier.
type Sport string

const (
	SportFootball   Sport = "football"
	SportBasketball Sport = "basketball"
	SportBaseball   Sport = "baseball"
	SportHockey     Sport = "hockey"
	SportSoccer     Sport = "soccer"
)

// League represents a sports league from a provider.
type League struct {
	ID      string
	Name    string
	Country string
	Sport   Sport
}

// Team represents a team in a league.
type Team struct {
	ID       string
	Name     string
	LeagueID string
	LogoURL  string
}

// Event represents a scheduled or live sports game.
type Event struct {
	ID          string
	HomeTeamID  string
	AwayTeamID  string
	LeagueID    string
	StartTime   time.Time
	HomeScore   int
	AwayScore   int
	Status      string
	VenueName   string
	BroadcastCh string
	Season      string
	Week        int
}

// Provider is the interface all sports data adapters must implement.
type Provider interface {
	// Name returns the provider identifier (e.g. "apifootball", "thesportsdb").
	Name() string

	// ListLeagues returns leagues for the given sport.
	ListLeagues(ctx context.Context, sport Sport) ([]League, error)

	// ListTeams returns teams in a league by its external league ID.
	ListTeams(ctx context.Context, leagueID string) ([]Team, error)

	// ListEvents returns events in a league for the date range [from, to].
	ListEvents(ctx context.Context, leagueID string, from, to time.Time) ([]Event, error)

	// LiveScore returns the current live state of an event.
	LiveScore(ctx context.Context, eventID string) (*Event, error)
}
