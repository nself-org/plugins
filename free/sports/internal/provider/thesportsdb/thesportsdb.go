// Package thesportsdb implements the provider.Provider interface using
// TheSportsDB API v2.
//
// Required env var: SPORTS_THESPORTSDB_TOKEN
// Optional env var: SPORTS_THESPORTSDB_HOST (default: www.thesportsdb.com)
package thesportsdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/nself-org/nself-sports/internal/provider"
)

const defaultHost = "www.thesportsdb.com"

// Client is the TheSportsDB adapter.
type Client struct {
	token string
	host  string
	http  *http.Client
}

// New returns a TheSportsDB client reading the token from env.
func New() (*Client, error) {
	token := os.Getenv("SPORTS_THESPORTSDB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SPORTS_THESPORTSDB_TOKEN is not set")
	}
	host := os.Getenv("SPORTS_THESPORTSDB_HOST")
	if host == "" {
		host = defaultHost
	}
	return &Client{
		token: token,
		host:  host,
		http:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *Client) Name() string { return "thesportsdb" }

func (c *Client) get(ctx context.Context, path string, out any) error {
	url := fmt.Sprintf("https://%s/api/v2/json/%s%s", c.host, c.token, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("thesportsdb GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("thesportsdb GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// --- Provider interface ---------------------------------------------------------

func (c *Client) ListLeagues(ctx context.Context, sport provider.Sport) ([]provider.League, error) {
	// Map our sport constant to TheSportsDB sport name.
	sportName := sportToTSDB(sport)
	var resp struct {
		Leagues []struct {
			IDLeague    string `json:"idLeague"`
			StrLeague   string `json:"strLeague"`
			StrCountry  string `json:"strCountry"`
		} `json:"leagues"`
	}
	if err := c.get(ctx, fmt.Sprintf("/search_all_leagues.php?s=%s", sportName), &resp); err != nil {
		return nil, err
	}
	leagues := make([]provider.League, 0, len(resp.Leagues))
	for _, l := range resp.Leagues {
		leagues = append(leagues, provider.League{
			ID:      l.IDLeague,
			Name:    l.StrLeague,
			Country: l.StrCountry,
			Sport:   sport,
		})
	}
	return leagues, nil
}

func (c *Client) ListTeams(ctx context.Context, leagueID string) ([]provider.Team, error) {
	var resp struct {
		Teams []struct {
			IDTeam   string `json:"idTeam"`
			StrTeam  string `json:"strTeam"`
			StrBadge string `json:"strBadge"`
		} `json:"teams"`
	}
	if err := c.get(ctx, fmt.Sprintf("/lookup_all_teams.php?id=%s", leagueID), &resp); err != nil {
		return nil, err
	}
	teams := make([]provider.Team, 0, len(resp.Teams))
	for _, t := range resp.Teams {
		teams = append(teams, provider.Team{
			ID:       t.IDTeam,
			Name:     t.StrTeam,
			LeagueID: leagueID,
			LogoURL:  t.StrBadge,
		})
	}
	return teams, nil
}

func (c *Client) ListEvents(ctx context.Context, leagueID string, from, to time.Time) ([]provider.Event, error) {
	// TheSportsDB v2: events_by_league_season or events_day
	// We use events_day per day; for a range, just use the from date.
	return c.fetchEventsByDate(ctx, leagueID, from)
}

func (c *Client) LiveScore(ctx context.Context, eventID string) (*provider.Event, error) {
	var resp struct {
		Event []struct {
			IDEvent     string `json:"idEvent"`
			IDLeague    string `json:"idLeague"`
			IDHomeTeam  string `json:"idHomeTeam"`
			IDAwayTeam  string `json:"idAwayTeam"`
			DateEvent   string `json:"dateEvent"`
			StrTime     string `json:"strTime"`
			IntHomeScore string `json:"intHomeScore"`
			IntAwayScore string `json:"intAwayScore"`
			StrStatus    string `json:"strStatus"`
			StrVenue     string `json:"strVenue"`
			IntSeason    string `json:"intSeason"`
		} `json:"event"`
	}
	if err := c.get(ctx, fmt.Sprintf("/lookupevent.php?id=%s", eventID), &resp); err != nil {
		return nil, err
	}
	if len(resp.Event) == 0 {
		return nil, fmt.Errorf("thesportsdb: no event found for id %s", eventID)
	}
	e := parseEvent(resp.Event[0].IDEvent,
		resp.Event[0].IDLeague,
		resp.Event[0].IDHomeTeam,
		resp.Event[0].IDAwayTeam,
		resp.Event[0].DateEvent,
		resp.Event[0].StrTime,
		resp.Event[0].IntHomeScore,
		resp.Event[0].IntAwayScore,
		resp.Event[0].StrStatus,
		resp.Event[0].StrVenue,
		resp.Event[0].IntSeason,
	)
	return &e, nil
}

func (c *Client) fetchEventsByDate(ctx context.Context, leagueID string, date time.Time) ([]provider.Event, error) {
	var resp struct {
		Events []struct {
			IDEvent      string `json:"idEvent"`
			IDLeague     string `json:"idLeague"`
			IDHomeTeam   string `json:"idHomeTeam"`
			IDAwayTeam   string `json:"idAwayTeam"`
			DateEvent    string `json:"dateEvent"`
			StrTime      string `json:"strTime"`
			IntHomeScore string `json:"intHomeScore"`
			IntAwayScore string `json:"intAwayScore"`
			StrStatus    string `json:"strStatus"`
			StrVenue     string `json:"strVenue"`
			IntSeason    string `json:"intSeason"`
		} `json:"events"`
	}
	if err := c.get(ctx, fmt.Sprintf("/eventsday.php?d=%s&l=%s", date.Format("2006-01-02"), leagueID), &resp); err != nil {
		return nil, err
	}
	events := make([]provider.Event, 0, len(resp.Events))
	for _, e := range resp.Events {
		events = append(events, parseEvent(
			e.IDEvent, e.IDLeague, e.IDHomeTeam, e.IDAwayTeam,
			e.DateEvent, e.StrTime, e.IntHomeScore, e.IntAwayScore,
			e.StrStatus, e.StrVenue, e.IntSeason,
		))
	}
	return events, nil
}

func parseEvent(id, leagueID, homeID, awayID, dateStr, timeStr, homeScore, awayScore, status, venue, season string) provider.Event {
	t, _ := time.Parse("2006-01-02 15:04:05", dateStr+" "+timeStr)
	home, _ := strconv.Atoi(homeScore)
	away, _ := strconv.Atoi(awayScore)
	return provider.Event{
		ID:         id,
		LeagueID:   leagueID,
		HomeTeamID: homeID,
		AwayTeamID: awayID,
		StartTime:  t,
		HomeScore:  home,
		AwayScore:  away,
		Status:     status,
		VenueName:  venue,
		Season:     season,
	}
}

func sportToTSDB(s provider.Sport) string {
	switch s {
	case provider.SportFootball:
		return "American+Football"
	case provider.SportBasketball:
		return "Basketball"
	case provider.SportBaseball:
		return "Baseball"
	case provider.SportHockey:
		return "Ice+Hockey"
	case provider.SportSoccer:
		return "Soccer"
	default:
		return string(s)
	}
}
