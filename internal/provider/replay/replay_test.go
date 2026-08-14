package replay

import (
	"os"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/live"
)

var start = time.Date(2026, 7, 27, 23, 0, 0, 0, time.UTC)

func newProvider(t *testing.T, clock *time.Time, speed float64) *Provider {
	t.Helper()

	p, err := New(os.DirFS("testdata"), speed,
		WithStart(start),
		WithClock(func() time.Time { return *clock }),
	)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return p
}

func gameAt(t *testing.T, p *Provider) core.Game {
	t.Helper()

	games, err := p.Games(t.Context(), core.LeagueWNBA, core.DayOf(start))
	if err != nil {
		t.Fatalf("games: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("want 1 game, got %d", len(games))
	}
	return games[0]
}

func TestGameProgressesWithVirtualTime(t *testing.T) {
	now := start
	p := newProvider(t, &now, 1)

	tests := []struct {
		name       string
		after      time.Duration
		wantStatus core.GameStatus
		wantHome   int
		wantAway   int
	}{
		{name: "before tip-off", after: -time.Minute, wantStatus: core.GameScheduled},
		{name: "first quarter", after: 400 * time.Second, wantStatus: core.GameLive, wantAway: 6},
		{name: "third quarter", after: 2000 * time.Second, wantStatus: core.GameLive, wantHome: 8, wantAway: 17},
		{name: "final", after: 3050 * time.Second, wantStatus: core.GameFinal, wantHome: 22, wantAway: 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now = start.Add(tt.after)
			game := gameAt(t, p)

			if game.Status != tt.wantStatus {
				t.Errorf("status = %s, want %s", game.Status, tt.wantStatus)
			}
			if game.Home.Score != tt.wantHome || game.Away.Score != tt.wantAway {
				t.Errorf("score = %d:%d, want %d:%d",
					game.Home.Score, game.Away.Score, tt.wantHome, tt.wantAway)
			}
		})
	}
}

func TestSpeedCompressesTheGame(t *testing.T) {
	now := start
	p := newProvider(t, &now, 60)

	now = start.Add(51 * time.Second)
	if game := gameAt(t, p); game.Status != core.GameFinal {
		t.Errorf("at 60x the game should be over after 51s, got %s", game.Status)
	}
}

func TestPlaysAreRevealedGradually(t *testing.T) {
	now := start.Add(1000 * time.Second)
	p := newProvider(t, &now, 1)

	feed, err := p.Plays(t.Context(), gameAt(t, p), "")
	if err != nil {
		t.Fatalf("plays: %v", err)
	}
	if len(feed.Plays) != 5 {
		t.Fatalf("want 5 plays after 1000s, got %d", len(feed.Plays))
	}
	if feed.Cursor != "5" {
		t.Errorf("cursor = %q, want %q", feed.Cursor, "5")
	}

	now = start.Add(3050 * time.Second)
	feed, err = p.Plays(t.Context(), gameAt(t, p), feed.Cursor)
	if err != nil {
		t.Fatalf("plays: %v", err)
	}
	if len(feed.Plays) != 16 {
		t.Errorf("want the full feed of 16 plays, got %d", len(feed.Plays))
	}
}

func TestTeamsFromFixtures(t *testing.T) {
	now := start
	p := newProvider(t, &now, 1)

	teams, err := p.Teams(t.Context(), core.LeagueWNBA)
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("want 2 teams, got %d", len(teams))
	}
	for _, team := range teams {
		if team.ExternalID == "" || team.Team.Name == "" {
			t.Errorf("incomplete team: %+v", team)
		}
	}
}

func TestReplayFeedsTheDetector(t *testing.T) {
	now := start.Add(3050 * time.Second)
	p := newProvider(t, &now, 1)

	game := gameAt(t, p)
	feed, err := p.Plays(t.Context(), game, "")
	if err != nil {
		t.Fatalf("plays: %v", err)
	}

	events := live.Detect(game, feed.Plays)

	want := map[core.EventKind]int{
		core.EventGameStarted: 1,
		core.EventGameFinal:   1,
		core.EventRun:         1,
		core.EventLeadChange:  1,
		core.EventCloseFinish: 1,
	}
	got := make(map[core.EventKind]int)
	for _, e := range events {
		got[e.Kind]++
	}

	for kind, n := range want {
		if got[kind] != n {
			t.Errorf("%s: got %d events, want %d", kind, got[kind], n)
		}
	}
	for kind, n := range got {
		if want[kind] == 0 {
			t.Errorf("unexpected event kind %s (%d)", kind, n)
		}
	}

	for _, e := range events {
		if e.Kind != core.EventRun {
			continue
		}
		if e.Run.Team != "NYL" || e.Run.Points != 12 || e.Run.Against != 2 {
			t.Errorf("unexpected run: %+v", e.Run)
		}
	}
}

func TestReplayFeedsTheAggregator(t *testing.T) {
	now := start.Add(3050 * time.Second)
	p := newProvider(t, &now, 1)

	game := gameAt(t, p)
	box, err := p.BoxScore(t.Context(), game)
	if err != nil {
		t.Fatalf("box score: %v", err)
	}
	feed, err := p.Plays(t.Context(), game, "")
	if err != nil {
		t.Fatalf("plays: %v", err)
	}

	stats := live.Aggregate(box, feed.Plays)

	if stats.ClutchMargin != 9 {
		t.Errorf("clutch margin = %d, want 9", stats.ClutchMargin)
	}

	clutch, ok := stats.ClutchLeader()
	if !ok {
		t.Fatal("clutch leader should be visible when the margin was 9")
	}
	if clutch.Player.Name != "Sabrina Ionescu" || clutch.ClutchPoints != 8 {
		t.Errorf("clutch leader = %s with %d points, want Sabrina Ionescu with 8",
			clutch.Player.Name, clutch.ClutchPoints)
	}

	rebounder, ok := stats.TopRebounder()
	if !ok || rebounder.Player.Name != "A'ja Wilson" {
		t.Errorf("top rebounder = %+v", rebounder)
	}

	nyl := stats.ByTeam("NYL").TopScorers(2)
	if len(nyl) != 2 || nyl[0].Player.Name != "Sabrina Ionescu" {
		t.Errorf("NYL top scorers = %+v", nyl)
	}
}
