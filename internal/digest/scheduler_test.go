package digest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func moscow(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

type fakeStore struct {
	subs      []core.Subscriber
	subsOf    map[core.SubscriberID][]core.Subscription
	games     []core.Game
	stats     map[core.GameID]core.GameStats
	processed map[core.Day]map[core.SubscriberID]bool
	marked    []core.Day
	sentFlags []bool
	failGames bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		subsOf:    make(map[core.SubscriberID][]core.Subscription),
		stats:     make(map[core.GameID]core.GameStats),
		processed: make(map[core.Day]map[core.SubscriberID]bool),
	}
}

func (f *fakeStore) AllSubscribers(context.Context) ([]core.Subscriber, error) {
	return f.subs, nil
}

func (f *fakeStore) SubscriptionsOf(_ context.Context, id core.SubscriberID) ([]core.Subscription, error) {
	return f.subsOf[id], nil
}

func (f *fakeStore) ProcessedDigests(_ context.Context, day core.Day) (map[core.SubscriberID]bool, error) {
	return f.processed[day], nil
}

func (f *fakeStore) MarkDigestProcessed(_ context.Context, id core.SubscriberID, day core.Day, sent bool) (bool, error) {
	if f.processed[day] == nil {
		f.processed[day] = make(map[core.SubscriberID]bool)
	}
	if f.processed[day][id] {
		return false, nil
	}
	f.processed[day][id] = true
	f.marked = append(f.marked, day)
	f.sentFlags = append(f.sentFlags, sent)
	return true, nil
}

func (f *fakeStore) GamesByDay(context.Context, core.League, core.Day, *time.Location) ([]core.Game, error) {
	if f.failGames {
		return nil, errors.New("database is down")
	}
	return f.games, nil
}

func (f *fakeStore) GameStats(context.Context, []core.GameID) (map[core.GameID]core.GameStats, error) {
	return f.stats, nil
}

type fakeSender struct {
	digests []core.Digest
	chats   []int64
}

func (s *fakeSender) SendDigest(_ context.Context, chatID int64, d core.Digest) error {
	s.chats = append(s.chats, chatID)
	s.digests = append(s.digests, d)
	return nil
}

func finalGame(id core.GameID, home, away core.TeamCode, hs, as int) core.Game {
	return core.Game{
		ID:     id,
		League: core.LeagueWNBA,
		Status: core.GameFinal,
		Home:   core.TeamScore{Team: core.Team{League: core.LeagueWNBA, Code: home, Name: string(home)}, Score: hs},
		Away:   core.TeamScore{Team: core.Team{League: core.LeagueWNBA, Code: away, Name: string(away)}, Score: as},
	}
}

func newScheduler(store Store, sender Sender) *Scheduler {
	return New(store, sender, core.LeagueWNBA, WithLogger(quiet()))
}

func TestDigestGoesOutAtTheLocalTime(t *testing.T) {
	loc := moscow(t)
	store := newFakeStore()
	store.subs = []core.Subscriber{{
		ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8},
	}}
	store.games = []core.Game{finalGame("wnba:1", "NYL", "LVA", 88, 81)}

	sender := &fakeSender{}
	s := newScheduler(store, sender)

	// 04:00 UTC is 07:00 in Moscow — too early.
	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 4, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("early tick: %v", err)
	}
	if len(sender.digests) != 0 {
		t.Fatalf("digest sent an hour early")
	}

	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 5, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(sender.digests) != 1 || sender.chats[0] != 100 {
		t.Fatalf("digest not delivered: %+v", sender.chats)
	}

	want := core.Day{Year: 2026, Month: time.August, Day: 10}
	if got := sender.digests[0].Day; got != want {
		t.Errorf("digest is for %s, want yesterday %s", got, want)
	}
}

func TestDigestIsSentOnlyOncePerDay(t *testing.T) {
	loc := moscow(t)
	store := newFakeStore()
	store.subs = []core.Subscriber{{ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8}}}
	store.games = []core.Game{finalGame("wnba:1", "NYL", "LVA", 88, 81)}

	sender := &fakeSender{}
	s := newScheduler(store, sender)
	at := time.Date(2026, 8, 11, 5, 0, 0, 0, time.UTC)

	for range 3 {
		if err := s.Tick(t.Context(), at); err != nil {
			t.Fatalf("tick: %v", err)
		}
	}
	if len(sender.digests) != 1 {
		t.Errorf("digest sent %d times", len(sender.digests))
	}
}

func TestEmptyDigestIsMarkedButNotSent(t *testing.T) {
	loc := moscow(t)
	store := newFakeStore()
	store.subs = []core.Subscriber{{ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8}}}

	sender := &fakeSender{}
	s := newScheduler(store, sender)

	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 5, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(sender.digests) != 0 {
		t.Error("nothing to report, so nothing should be sent")
	}
	if len(store.marked) != 1 || store.sentFlags[0] {
		t.Errorf("the day must still be marked as processed: %+v %v", store.marked, store.sentFlags)
	}
}

func TestSubscribersInDifferentZonesGetTheirOwnMoment(t *testing.T) {
	loc := moscow(t)
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	store := newFakeStore()
	store.subs = []core.Subscriber{
		{ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8}},
		{ID: 2, ChatID: 200, Timezone: newYork, DigestAt: core.DailyTime{Hour: 8}},
	}
	store.games = []core.Game{finalGame("wnba:1", "NYL", "LVA", 88, 81)}

	sender := &fakeSender{}
	s := newScheduler(store, sender)

	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 5, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("moscow tick: %v", err)
	}
	if len(sender.chats) != 1 || sender.chats[0] != 100 {
		t.Fatalf("only the Moscow subscriber should be served, got %v", sender.chats)
	}

	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("new york tick: %v", err)
	}
	if len(sender.chats) != 2 || sender.chats[1] != 200 {
		t.Errorf("New York subscriber missed their slot: %v", sender.chats)
	}
}

func TestBuildSplitsFeaturedAndRest(t *testing.T) {
	sub := core.Subscriber{ID: 1}
	subscriptions := []core.Subscription{{SubscriberID: 1, League: core.LeagueWNBA, Team: "NYL"}}

	games := []core.Game{
		finalGame("wnba:1", "NYL", "LVA", 88, 81),
		finalGame("wnba:2", "SEA", "ATL", 70, 75),
		{ID: "wnba:3", League: core.LeagueWNBA, Status: core.GameScheduled},
	}

	built := Build(sub, subscriptions, games, nil, core.Day{Year: 2026, Month: time.August, Day: 10})

	if len(built.Games) != 2 {
		t.Fatalf("unfinished games must be left out, got %d", len(built.Games))
	}
	featured := built.FeaturedGames()
	if len(featured) != 1 || featured[0].Game.ID != "wnba:1" {
		t.Errorf("featured = %+v", featured)
	}
	if rest := built.RestGames(); len(rest) != 1 || rest[0].Game.ID != "wnba:2" {
		t.Errorf("rest = %+v", rest)
	}
}

func TestStoreFailureDoesNotMarkTheDay(t *testing.T) {
	loc := moscow(t)
	store := newFakeStore()
	store.subs = []core.Subscriber{{ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8}}}
	store.failGames = true

	s := newScheduler(store, &fakeSender{})
	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 5, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("tick should survive a failing store: %v", err)
	}
	if len(store.marked) != 0 {
		t.Error("a failed digest must stay unmarked so the next tick retries it")
	}
}

func TestDigestSurvivesAMissedMinute(t *testing.T) {
	loc := moscow(t)
	store := newFakeStore()
	store.subs = []core.Subscriber{{ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8}}}
	store.games = []core.Game{finalGame("wnba:1", "NYL", "LVA", 88, 81)}

	sender := &fakeSender{}
	s := newScheduler(store, sender)

	// 06:20 UTC is 09:20 in Moscow: the 08:00 tick was missed, but we are still in the window.
	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 6, 20, 0, 0, time.UTC)); err != nil {
		t.Fatalf("late tick: %v", err)
	}
	if len(sender.digests) != 1 {
		t.Fatalf("a missed minute must not lose the digest, got %d", len(sender.digests))
	}
}

func TestDigestIsNotSentHoursLate(t *testing.T) {
	loc := moscow(t)
	store := newFakeStore()
	store.subs = []core.Subscriber{{ID: 1, ChatID: 100, Timezone: loc, DigestAt: core.DailyTime{Hour: 8}}}
	store.games = []core.Game{finalGame("wnba:1", "NYL", "LVA", 88, 81)}

	sender := &fakeSender{}
	s := newScheduler(store, sender)

	// 17:00 UTC is 20:00 in Moscow — twelve hours late, nobody needs it now.
	if err := s.Tick(t.Context(), time.Date(2026, 8, 11, 17, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("very late tick: %v", err)
	}
	if len(sender.digests) != 0 {
		t.Errorf("a twelve-hour-late digest should be skipped, got %d", len(sender.digests))
	}
}
