package postgres

import (
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func TestSaveAndReadGameStats(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedGame(t, store, "wnba:1")

	stats := core.GameStats{
		GameID:       "wnba:1",
		League:       core.LeagueWNBA,
		ClutchMargin: 4,
		Lines: []core.PlayerLine{
			{Player: core.Player{ID: "p1", Name: "Sabrina Ionescu", Team: "NYL"}, Points: 31, Rebounds: 5, Assists: 9, ClutchPoints: 12},
			{Player: core.Player{ID: "p2", Name: "Breanna Stewart", Team: "NYL"}, Points: 24, Rebounds: 11, Assists: 3, ClutchPoints: 4},
			{Player: core.Player{ID: "p3", Name: "A'ja Wilson", Team: "LVA"}, Points: 28, Rebounds: 14, Assists: 2, ClutchPoints: 6},
		},
	}
	if err := store.SaveGameStats(ctx, stats); err != nil {
		t.Fatalf("save: %v", err)
	}

	stats.Lines[0].Points = 33
	if err := store.SaveGameStats(ctx, stats); err != nil {
		t.Fatalf("resave: %v", err)
	}

	got, err := store.GameStats(ctx, []core.GameID{"wnba:1"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	stored, ok := got["wnba:1"]
	if !ok {
		t.Fatal("stats not found")
	}
	if len(stored.Lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(stored.Lines))
	}
	if stored.ClutchMargin != 4 {
		t.Errorf("clutch margin: %d", stored.ClutchMargin)
	}

	top := stored.TopScorers(2)
	if len(top) != 2 || top[0].Points != 33 || top[1].Points != 28 {
		t.Errorf("unexpected top scorers: %+v", top)
	}

	nyl := stored.ByTeam("NYL")
	rebounder, ok := nyl.TopRebounder()
	if !ok || rebounder.Player.Name != "Breanna Stewart" {
		t.Errorf("unexpected NYL rebounder: %+v", rebounder)
	}

	clutch, ok := stored.ClutchLeader()
	if !ok || clutch.Player.ID != "p1" {
		t.Errorf("unexpected clutch leader: %+v", clutch)
	}
}

func TestClutchLeaderHiddenInBlowout(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedGame(t, store, "wnba:1")

	stats := core.GameStats{
		GameID:       "wnba:1",
		League:       core.LeagueWNBA,
		ClutchMargin: 24,
		Lines: []core.PlayerLine{
			{Player: core.Player{ID: "p1", Name: "Bench Player", Team: "NYL"}, Points: 8, ClutchPoints: 8},
		},
	}
	if err := store.SaveGameStats(ctx, stats); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.GameStats(ctx, []core.GameID{"wnba:1"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, ok := got["wnba:1"].ClutchLeader(); ok {
		t.Error("clutch leader should be hidden when the margin was 24")
	}
}

func TestDigestRunsAreClaimedOnce(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	sub, err := store.EnsureSubscriber(ctx, 1)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	day := core.Day{Year: 2026, Month: time.July, Day: 27}

	claimed, err := store.MarkDigestProcessed(ctx, sub.ID, day, true)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !claimed {
		t.Fatal("first call should claim the day")
	}

	claimed, err = store.MarkDigestProcessed(ctx, sub.ID, day, true)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if claimed {
		t.Error("second call claimed the same day again")
	}

	processed, err := store.ProcessedDigests(ctx, day)
	if err != nil {
		t.Fatalf("processed: %v", err)
	}
	if len(processed) != 1 || !processed[sub.ID] {
		t.Errorf("unexpected processed map: %+v", processed)
	}
}
