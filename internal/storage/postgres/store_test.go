package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/damnary/nba-digest/internal/core"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("nba_digest"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "start postgres container: %v\n", err)
		os.Exit(1)
	}

	testDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection string: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	_ = testcontainers.TerminateContainer(container)
	os.Exit(code)
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := t.Context()

	store, err := New(ctx, testDSN)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(store.Close)

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	_, err = store.pool.Exec(ctx, `TRUNCATE teams, subscribers, games RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return store
}

func seedTeams(t *testing.T, store *Store) {
	t.Helper()
	teams := []core.Team{
		{League: core.LeagueWNBA, Code: "NYL", Name: "New York Liberty"},
		{League: core.LeagueWNBA, Code: "LVA", Name: "Las Vegas Aces"},
		{League: core.LeagueWNBA, Code: "SEA", Name: "Seattle Storm"},
	}
	if err := store.UpsertTeams(t.Context(), teams); err != nil {
		t.Fatalf("upsert teams: %v", err)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestUpsertTeamsOverwritesName(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	renamed := []core.Team{{League: core.LeagueWNBA, Code: "NYL", Name: "Liberty"}}
	if err := store.UpsertTeams(ctx, renamed); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	teams, err := store.Teams(ctx, core.LeagueWNBA)
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	if len(teams) != 3 {
		t.Fatalf("want 3 teams, got %d", len(teams))
	}
	for _, team := range teams {
		if team.Code == "NYL" && team.Name != "Liberty" {
			t.Errorf("name not updated: %q", team.Name)
		}
	}
}

func TestTeamByExternalID(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	nyl := core.Team{League: core.LeagueWNBA, Code: "NYL"}
	if err := store.LinkTeamExternalID(ctx, "espn", "9", nyl); err != nil {
		t.Fatalf("link: %v", err)
	}

	got, err := store.TeamByExternalID(ctx, "espn", "9")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Code != "NYL" || got.Name != "New York Liberty" {
		t.Errorf("unexpected team: %+v", got)
	}

	_, err = store.TeamByExternalID(ctx, "espn", "404")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestEnsureSubscriberIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	first, err := store.EnsureSubscriber(ctx, 100500)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := store.EnsureSubscriber(ctx, 100500)
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if first.ID != second.ID {
		t.Errorf("ids differ: %d vs %d", first.ID, second.ID)
	}
	if first.Timezone.String() != "Europe/Moscow" {
		t.Errorf("unexpected timezone: %s", first.Timezone)
	}
	if first.DigestAt != (core.DailyTime{Hour: 8}) {
		t.Errorf("unexpected digest time: %s", first.DigestAt)
	}
	if !first.AlertsOn {
		t.Error("alerts should be on by default")
	}
}

func TestSubscriberSettingsRoundTrip(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	sub, err := store.EnsureSubscriber(ctx, 42)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	tz, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	if err := store.SetTimezone(ctx, sub.ID, tz); err != nil {
		t.Fatalf("set timezone: %v", err)
	}
	if err := store.SetDigestAt(ctx, sub.ID, core.DailyTime{Hour: 9, Minute: 30}); err != nil {
		t.Fatalf("set digest at: %v", err)
	}
	if err := store.SetAlerts(ctx, sub.ID, false); err != nil {
		t.Fatalf("set alerts: %v", err)
	}

	got, err := store.SubscriberByChat(ctx, 42)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.Timezone.String() != "America/New_York" {
		t.Errorf("timezone: %s", got.Timezone)
	}
	if got.DigestAt != (core.DailyTime{Hour: 9, Minute: 30}) {
		t.Errorf("digest at: %s", got.DigestAt)
	}
	if got.AlertsOn {
		t.Error("alerts should be off")
	}
}

func TestSubscribersForTeams(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	liberty, err := store.EnsureSubscriber(ctx, 1)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	storm, err := store.EnsureSubscriber(ctx, 2)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}

	if err := store.AddSubscription(ctx, liberty.ID, core.LeagueWNBA, "NYL"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := store.AddSubscription(ctx, liberty.ID, core.LeagueWNBA, "LVA"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := store.AddSubscription(ctx, storm.ID, core.LeagueWNBA, "SEA"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	got, err := store.SubscribersForTeams(ctx, core.LeagueWNBA, []core.TeamCode{"NYL", "LVA"})
	if err != nil {
		t.Fatalf("fan-out query: %v", err)
	}
	if len(got) != 1 || got[0].ID != liberty.ID {
		t.Fatalf("want only subscriber %d, got %+v", liberty.ID, got)
	}

	subs, err := store.SubscriptionsOf(ctx, liberty.ID)
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	if len(subs) != 2 {
		t.Errorf("want 2 subscriptions, got %d", len(subs))
	}
}

func TestAddSubscriptionIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	sub, err := store.EnsureSubscriber(ctx, 7)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	for range 3 {
		if err := store.AddSubscription(ctx, sub.ID, core.LeagueWNBA, "NYL"); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}

	subs, err := store.SubscriptionsOf(ctx, sub.ID)
	if err != nil {
		t.Fatalf("subscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Errorf("want 1 subscription, got %d", len(subs))
	}
}

func TestUnknownTeamIsRejected(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedTeams(t, store)

	sub, err := store.EnsureSubscriber(ctx, 8)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := store.AddSubscription(ctx, sub.ID, core.LeagueWNBA, "ZZZ"); err == nil {
		t.Fatal("expected foreign key violation")
	}
}
