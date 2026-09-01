package internal_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/nself-sports/internal"
	"github.com/nself-org/nself-sports/internal/provider"
)

// mockProvider implements provider.Provider with configurable responses.
type mockProvider struct {
	name    string
	leagues []provider.League
	teams   []provider.Team
	events  []provider.Event
	live    *provider.Event
	err     error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) ListLeagues(_ context.Context, _ provider.Sport) ([]provider.League, error) {
	return m.leagues, m.err
}

func (m *mockProvider) ListTeams(_ context.Context, _ string) ([]provider.Team, error) {
	return m.teams, m.err
}

func (m *mockProvider) ListEvents(_ context.Context, _ string, _, _ time.Time) ([]provider.Event, error) {
	return m.events, m.err
}

func (m *mockProvider) LiveScore(_ context.Context, _ string) (*provider.Event, error) {
	return m.live, m.err
}

// --- Provider interface tests (no DB) -------------------------------------------

func TestMockProviderListLeagues(t *testing.T) {
	mp := &mockProvider{
		name: "mock",
		leagues: []provider.League{
			{ID: "39", Name: "Premier League", Country: "England", Sport: provider.SportSoccer},
		},
	}
	leagues, err := mp.ListLeagues(context.Background(), provider.SportSoccer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(leagues) != 1 {
		t.Fatalf("expected 1 league, got %d", len(leagues))
	}
	if leagues[0].ID != "39" {
		t.Errorf("expected league ID '39', got %q", leagues[0].ID)
	}
	if leagues[0].Sport != provider.SportSoccer {
		t.Errorf("expected sport soccer, got %q", leagues[0].Sport)
	}
}

func TestMockProviderListTeams(t *testing.T) {
	mp := &mockProvider{
		name: "mock",
		teams: []provider.Team{
			{ID: "33", Name: "Manchester United", LeagueID: "39", LogoURL: "https://example.com/mu.png"},
			{ID: "34", Name: "Newcastle United", LeagueID: "39"},
		},
	}
	teams, err := mp.ListTeams(context.Background(), "39")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
}

func TestMockProviderListEvents(t *testing.T) {
	now := time.Now().UTC()
	mp := &mockProvider{
		name: "mock",
		events: []provider.Event{
			{
				ID:         "fixture-1",
				LeagueID:   "39",
				HomeTeamID: "33",
				AwayTeamID: "34",
				StartTime:  now,
				Status:     "scheduled",
			},
		},
	}
	events, err := mp.ListEvents(context.Background(), "39", now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].ID != "fixture-1" {
		t.Errorf("expected event ID 'fixture-1', got %q", events[0].ID)
	}
}

func TestMockProviderLiveScore(t *testing.T) {
	ev := &provider.Event{
		ID:        "fixture-1",
		HomeScore: 2,
		AwayScore: 1,
		Status:    "1H",
	}
	mp := &mockProvider{name: "mock", live: ev}
	got, err := mp.LiveScore(context.Background(), "fixture-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.HomeScore != 2 {
		t.Errorf("expected HomeScore 2, got %d", got.HomeScore)
	}
	if got.AwayScore != 1 {
		t.Errorf("expected AwayScore 1, got %d", got.AwayScore)
	}
}

// --- Syncer nil-pool guard tests ---

func TestNewSyncer_DefaultAccount(t *testing.T) {
	mp := &mockProvider{name: "mock"}
	s := internal.NewSyncer(nil, mp, "")
	if s == nil {
		t.Fatal("NewSyncer returned nil")
	}
}

// --- Sport constants ---

func TestSportConstants(t *testing.T) {
	sports := []provider.Sport{
		provider.SportFootball,
		provider.SportBasketball,
		provider.SportBaseball,
		provider.SportHockey,
		provider.SportSoccer,
	}
	seen := map[provider.Sport]bool{}
	for _, s := range sports {
		if seen[s] {
			t.Errorf("duplicate sport constant: %q", s)
		}
		seen[s] = true
		if string(s) == "" {
			t.Errorf("empty sport constant value for %T", s)
		}
	}
}

// --- Error propagation ---

func TestMockProviderError(t *testing.T) {
	wantErr := "provider offline"
	mp := &mockProvider{name: "mock", err: errors.New(wantErr)}
	_, err := mp.ListLeagues(context.Background(), provider.SportSoccer)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != wantErr {
		t.Errorf("expected %q, got %q", wantErr, err.Error())
	}
}
