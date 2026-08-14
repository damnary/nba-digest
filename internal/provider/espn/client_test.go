package espn

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/live"
)

var observedAt = time.Date(2026, 8, 11, 6, 0, 0, 0, time.UTC)

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()

	serve := func(w http.ResponseWriter, name string) {
		raw, err := os.ReadFile("testdata/" + name)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/site/wnba/teams", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, "teams.json")
	})
	mux.HandleFunc("/site/wnba/scoreboard", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, "scoreboard.json")
	})
	mux.HandleFunc("/site/wnba/summary", func(w http.ResponseWriter, _ *http.Request) {
		serve(w, "summary.json")
	})
	mux.HandleFunc("/core/wnba/events/", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			serve(w, "plays_page1.json")
			return
		}
		serve(w, "plays_page2.json")
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T) *Client {
	t.Helper()
	srv := fixtureServer(t)
	return New(
		WithBaseURLs(srv.URL+"/site", srv.URL+"/core"),
		WithClock(func() time.Time { return observedAt }),
	)
}

func TestTeamsAreMapped(t *testing.T) {
	teams, err := newClient(t).Teams(t.Context(), core.LeagueWNBA)
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	if len(teams) != 15 {
		t.Fatalf("want 15 teams, got %d", len(teams))
	}

	byCode := make(map[core.TeamCode]core.ExternalTeam)
	for _, tm := range teams {
		byCode[tm.Team.Code] = tm
	}

	atl, ok := byCode["ATL"]
	if !ok {
		t.Fatal("ATL not found")
	}
	if atl.ExternalID != "20" || atl.Team.Name != "Atlanta Dream" || atl.Team.League != core.LeagueWNBA {
		t.Errorf("unexpected mapping: %+v", atl)
	}
}

func TestScoreboardIsMapped(t *testing.T) {
	games, err := newClient(t).Games(t.Context(), core.LeagueWNBA, core.Day{Year: 2026, Month: time.August, Day: 11})
	if err != nil {
		t.Fatalf("games: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("want 2 games, got %d", len(games))
	}

	g := games[0]
	if g.ID != "wnba:401857132" {
		t.Errorf("game id = %q", g.ID)
	}
	if g.Status != core.GameFinal {
		t.Errorf("status = %q, want final", g.Status)
	}
	if g.Home.Team.Code != "ATL" || g.Home.Score != 107 {
		t.Errorf("home = %+v", g.Home)
	}
	if g.Away.Team.Code != "TOR" || g.Away.Score != 95 {
		t.Errorf("away = %+v", g.Away)
	}
	if g.Clock != "" {
		t.Errorf("finished game should have no clock, got %q", g.Clock)
	}
	if !g.ObservedAt.Equal(observedAt) {
		t.Errorf("observedAt = %s", g.ObservedAt)
	}
	if g.StartsAt.IsZero() {
		t.Error("startsAt not parsed")
	}
}

func TestPlaysAreMappedAndPaginated(t *testing.T) {
	client := newClient(t)
	game := core.Game{ID: "wnba:401857132", League: core.LeagueWNBA}

	feed, err := client.Plays(t.Context(), game, "")
	if err != nil {
		t.Fatalf("plays: %v", err)
	}

	if feed.Cursor != "17" {
		t.Errorf("cursor = %q, want %q", feed.Cursor, "17")
	}
	if len(feed.Plays) == 0 {
		t.Fatal("no plays mapped")
	}

	var scoring core.Play
	for _, p := range feed.Plays {
		if p.Points > 0 {
			scoring = p
			break
		}
	}
	if scoring.Points == 0 {
		t.Fatal("no scoring play found")
	}
	if scoring.Team == "" {
		t.Error("team ref was not resolved to a code")
	}
	if scoring.Player.ID == "" {
		t.Error("athlete ref was not resolved to an id")
	}
	if scoring.OccurredAt.IsZero() {
		t.Error("wallclock was not parsed")
	}
	if scoring.Period == 0 || scoring.Clock == "" {
		t.Errorf("period/clock missing: %+v", scoring)
	}
}

func TestPlaysResumeFromCursor(t *testing.T) {
	client := newClient(t)
	game := core.Game{ID: "wnba:401857132", League: core.LeagueWNBA}

	feed, err := client.Plays(t.Context(), game, "17")
	if err != nil {
		t.Fatalf("plays: %v", err)
	}
	if len(feed.Plays) != 25 {
		t.Errorf("resuming from the last page should fetch it alone, got %d plays", len(feed.Plays))
	}
}

func TestBoxScoreIsMapped(t *testing.T) {
	client := newClient(t)
	game := core.Game{ID: "wnba:401857132", League: core.LeagueWNBA}

	stats, err := client.BoxScore(t.Context(), game)
	if err != nil {
		t.Fatalf("box score: %v", err)
	}
	if len(stats.Lines) == 0 {
		t.Fatal("no player lines mapped")
	}

	var conde core.PlayerLine
	for _, l := range stats.Lines {
		if l.Player.Name == "Maria Conde" {
			conde = l
			break
		}
	}
	if conde.Player.ID != "3910470" {
		t.Fatalf("Maria Conde not found in box score")
	}
	if conde.Points != 2 || conde.Rebounds != 1 || conde.Assists != 3 {
		t.Errorf("stats misread: %+v", conde)
	}
	if conde.Player.Team != "TOR" {
		t.Errorf("team = %q, want TOR", conde.Player.Team)
	}
}

func TestRealFeedRunsThroughTheDetector(t *testing.T) {
	client := newClient(t)

	games, err := client.Games(t.Context(), core.LeagueWNBA, core.Day{Year: 2026, Month: time.August, Day: 11})
	if err != nil {
		t.Fatalf("games: %v", err)
	}
	game := games[0]

	feed, err := client.Plays(t.Context(), game, "")
	if err != nil {
		t.Fatalf("plays: %v", err)
	}
	box, err := client.BoxScore(t.Context(), game)
	if err != nil {
		t.Fatalf("box score: %v", err)
	}

	events := live.Detect(game, feed.Plays)
	if len(events) == 0 {
		t.Fatal("detector produced nothing from a real feed")
	}
	for _, e := range events {
		if e.ID == "" || !strings.HasPrefix(string(e.ID), "wnba:401857132:") {
			t.Errorf("malformed event id: %q", e.ID)
		}
		if e.GameID != game.ID {
			t.Errorf("event points at the wrong game: %+v", e)
		}
	}

	stats := live.Aggregate(box, feed.Plays)
	if len(stats.Lines) != len(box.Lines) {
		t.Errorf("aggregate changed the roster size: %d vs %d", len(stats.Lines), len(box.Lines))
	}
}
