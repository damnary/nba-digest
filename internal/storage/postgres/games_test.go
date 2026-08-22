package postgres

import (
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func testGame(id core.GameID, status core.GameStatus, startsAt time.Time, home, away int) core.Game {
	return core.Game{
		ID:         id,
		League:     core.LeagueWNBA,
		StartsAt:   startsAt,
		Status:     status,
		Home:       core.TeamScore{Team: core.Team{League: core.LeagueWNBA, Code: "NYL"}, Score: home},
		Away:       core.TeamScore{Team: core.Team{League: core.LeagueWNBA, Code: "LVA"}, Score: away},
		Period:     4,
		Clock:      "02:14",
		ObservedAt: startsAt.Add(2 * time.Hour),
	}
}

func TestUpsertGamesUpdatesScore(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	start := time.Now().UTC().Truncate(time.Second)
	first := testGame("wnba:1", core.GameLive, start, 70, 64)
	if err := store.UpsertGames(ctx, []core.Game{first}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	second := testGame("wnba:1", core.GameFinal, start, 88, 81)
	second.ObservedAt = first.ObservedAt.Add(time.Minute)
	if err := store.UpsertGames(ctx, []core.Game{second}); err != nil {
		t.Fatalf("update: %v", err)
	}

	games, err := store.GamesWithin(ctx, core.LeagueWNBA, start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("within: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("want 1 game, got %d", len(games))
	}
	if games[0].Status != core.GameFinal || games[0].Home.Score != 88 {
		t.Errorf("unexpected game: %+v", games[0])
	}
	if games[0].Home.Team.Name != "New York Liberty" {
		t.Errorf("team name not joined: %q", games[0].Home.Team.Name)
	}
}

func TestUpsertGamesIgnoresStaleSnapshot(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	start := time.Now().UTC().Truncate(time.Second)
	fresh := testGame("wnba:1", core.GameFinal, start, 88, 81)
	if err := store.UpsertGames(ctx, []core.Game{fresh}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	stale := testGame("wnba:1", core.GameLive, start, 70, 64)
	stale.ObservedAt = fresh.ObservedAt.Add(-time.Minute)
	if err := store.UpsertGames(ctx, []core.Game{stale}); err != nil {
		t.Fatalf("stale upsert: %v", err)
	}

	games, err := store.ActiveGames(ctx, core.LeagueWNBA)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("stale snapshot resurrected the game: %+v", games)
	}
}

func TestActiveGamesExcludesFinalAndPostponed(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	start := time.Now().UTC().Truncate(time.Second)
	games := []core.Game{
		testGame("wnba:live", core.GameLive, start, 40, 38),
		testGame("wnba:scheduled", core.GameScheduled, start.Add(time.Hour), 0, 0),
		testGame("wnba:final", core.GameFinal, start.Add(-3*time.Hour), 88, 81),
		testGame("wnba:postponed", core.GamePostponed, start.Add(-time.Hour), 0, 0),
	}
	if err := store.UpsertGames(ctx, games); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	active, err := store.ActiveGames(ctx, core.LeagueWNBA)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("want 2 active games, got %d", len(active))
	}
	for _, g := range active {
		if !g.IsActive() {
			t.Errorf("inactive game returned: %s / %s", g.ID, g.Status)
		}
	}
}

func TestGamesWithinCoversTheOvernightWindow(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	tipoff := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if err := store.UpsertGames(ctx, []core.Game{testGame("wnba:1", core.GameFinal, tipoff, 88, 81)}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	digestAt := time.Date(2026, 8, 21, 8, 0, 0, 0, moscow)
	games, err := store.GamesWithin(ctx, core.LeagueWNBA, digestAt.Add(-24*time.Hour), digestAt)
	if err != nil {
		t.Fatalf("within: %v", err)
	}
	if len(games) != 1 {
		t.Errorf("a game from tonight must land in this morning digest window, got %d", len(games))
	}

	before, err := store.GamesWithin(ctx, core.LeagueWNBA, digestAt.Add(-48*time.Hour), digestAt.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("previous window: %v", err)
	}
	if len(before) != 0 {
		t.Errorf("the previous window must not contain tonight, got %d", len(before))
	}
}
