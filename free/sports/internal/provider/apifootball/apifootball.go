// Package apifootball implements the provider.Provider interface using
// the API-Football v3 REST API (api-sports.io).
//
// Required env var: SPORTS_APIFOOTBALL_TOKEN
// Optional env var: SPORTS_APIFOOTBALL_HOST (default: v3.football.api-sports.io)
package apifootball

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

const defaultHost = "v3.football.api-sports.io"

// Client is the API-Football adapter.
type Client struct {
	token  string
	host   string
	http   *http.Client
}

// New returns an API-Football client reading the token from env.
func New() (*Client, error) {
	token := os.Getenv("SPORTS_APIFOOTBALL_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SPORTS_APIFOOTBALL_TOKEN is not set")
	}
	host := os.Getenv("SPORTS_APIFOOTBALL_HOST")
	if host == "" {
		host = defaultHost
	}
	return &Client{
		token: token,
		host:  host,
		http:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *Client) Name() string { return "apifootball" }

// get performs an authenticated GET against the API-Football v3 endpoint.
func (c *Client) get(ctx context.Context, path string, out any) error {
	url := fmt.Sprintf("https://%s%s", c.host, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-apisports-key", c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("apifootball GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("apifootball GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// --- Provider interface ---------------------------------------------------------

func (c *Client) ListLeagues(ctx context.Context, sport provider.Sport) ([]provider.League, error) {
	// API-Football organises by "type" league/cup; we return all for the season year.
	var resp struct {
		Response []struct {
			League struct {
				ID      int    `json:"id"`
				Name    string `json:"name"`
				Country string `json:"country"`
			} `json:"league"`
			Country struct {
				Name string `json:"name"`
				Code string `json:"code"`
			} `json:"country"`
		} `json:"response"`
	}
	year := time.Now().Year()
	if err := c.get(ctx, fmt.Sprintf("/leagues?current=true&season=%d", year), &resp); err != nil {
		return nil, err
	}
	leagues := make([]provider.League, 0, len(resp.Response))
	for _, r := range resp.Response {
		leagues = append(leagues, provider.League{
			ID:      strconv.Itoa(r.League.ID),
			Name:    r.League.Name,
			Country: r.Country.Name,
			Sport:   sport,
		})
	}
	return leagues, nil
}

func (c *Client) ListTeams(ctx context.Context, leagueID string) ([]provider.Team, error) {
	year := time.Now().Year()
	var resp struct {
		Response []struct {
			Team struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
				Logo string `json:"logo"`
			} `json:"team"`
		} `json:"response"`
	}
	if err := c.get(ctx, fmt.Sprintf("/teams?league=%s&season=%d", leagueID, year), &resp); err != nil {
		return nil, err
	}
	teams := make([]provider.Team, 0, len(resp.Response))
	for _, r := range resp.Response {
		teams = append(teams, provider.Team{
			ID:       strconv.Itoa(r.Team.ID),
			Name:     r.Team.Name,
			LeagueID: leagueID,
			LogoURL:  r.Team.Logo,
		})
	}
	return teams, nil
}

func (c *Client) ListEvents(ctx context.Context, leagueID string, from, to time.Time) ([]provider.Event, error) {
	year := time.Now().Year()
	path := fmt.Sprintf("/fixtures?league=%s&season=%d&from=%s&to=%s",
		leagueID,
		year,
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
	)
	return c.fetchFixtures(ctx, path, leagueID)
}

func (c *Client) LiveScore(ctx context.Context, eventID string) (*provider.Event, error) {
	events, err := c.fetchFixtures(ctx, fmt.Sprintf("/fixtures?id=%s", eventID), "")
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("apifootball: no fixture found for id %s", eventID)
	}
	e := events[0]
	return &e, nil
}

// fetchFixtures is shared between ListEvents and LiveScore.
func (c *Client) fetchFixtures(ctx context.Context, path, leagueID string) ([]provider.Event, error) {
	var resp struct {
		Response []struct {
			Fixture struct {
				ID     int    `json:"id"`
				Date   string `json:"date"`
				Status struct {
					Short string `json:"short"`
				} `json:"status"`
				Venue struct {
					Name string `json:"name"`
				} `json:"venue"`
			} `json:"fixture"`
			League struct {
				ID     int    `json:"id"`
				Season int    `json:"season"`
				Round  string `json:"round"`
			} `json:"league"`
			Teams struct {
				Home struct {
					ID int `json:"id"`
				} `json:"home"`
				Away struct {
					ID int `json:"id"`
				} `json:"away"`
			} `json:"teams"`
			Goals struct {
				Home *int `json:"home"`
				Away *int `json:"away"`
			} `json:"goals"`
		} `json:"response"`
	}
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	events := make([]provider.Event, 0, len(resp.Response))
	for _, r := range resp.Response {
		t, _ := time.Parse(time.RFC3339, r.Fixture.Date)
		home := 0
		if r.Goals.Home != nil {
			home = *r.Goals.Home
		}
		away := 0
		if r.Goals.Away != nil {
			away = *r.Goals.Away
		}
		lid := leagueID
		if lid == "" {
			lid = strconv.Itoa(r.League.ID)
		}
		events = append(events, provider.Event{
			ID:         strconv.Itoa(r.Fixture.ID),
			HomeTeamID: strconv.Itoa(r.Teams.Home.ID),
			AwayTeamID: strconv.Itoa(r.Teams.Away.ID),
			LeagueID:   lid,
			StartTime:  t,
			HomeScore:  home,
			AwayScore:  away,
			Status:     r.Fixture.Status.Short,
			VenueName:  r.Fixture.Venue.Name,
			Season:     strconv.Itoa(r.League.Season),
		})
	}
	return events, nil
}
