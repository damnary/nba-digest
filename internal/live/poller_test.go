package live

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/platform/retry"
	"github.com/damnary/nba-digest/internal/provider/replay"
)

type fakeStore struct {
	mu        sync.Mutex
	games     map[core.GameID]core.Game
	events    map[core.EventID]core.Event
	stats     map[core.GameID]core.GameStats
	failNext  error
	failStats int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		games:  make(map[core.GameID]core.Game),
		events: make(map[core.EventID]core.Event),
		stats:  make(map[core.GameID]core.GameStats),
	}
}

func (s *fakeStore) UpsertGames(_ context.Context, games []core.Game) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, g := range games {
		s.games[g.ID] = g
	}
	return nil
}

func (s *fakeStore) SaveEvents(_ context.Context, events []core.Event) ([]core.EventID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failNext != nil {
		err := s.failNext
		s.failNext = nil
		return nil, err
	}

	var inserted []core.EventID
	for _, e := range events {
		if _, exists := s.events[e.ID]; exists {
			continue
		}
		s.events[e.ID] = e
		inserted = append(inserted, e.ID)
	}
	return inserted, nil
}

func (s *fakeStore) SaveGameStats(_ context.Context, stats core.GameStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failStats > 0 {
		s.failStats--
		return errors.New("stats storage is having a moment")
	}
	s.stats[stats.GameID] = stats
	return nil
}

func (s *fakeStore) counts() (games, events, stats int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.games), len(s.events), len(s.stats)
}

type fakePublisher struct {
	mu        sync.Mutex
	published []core.Event
}

func (p *fakePublisher) Publish(_ context.Context, e core.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, e)
	return nil
}

func (p *fakePublisher) snapshot() []core.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]core.Event(nil), p.published...)
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newReplayPoller(t *testing.T, store Store, pub Publisher, speed float64) *Poller {
	t.Helper()

	provider, err := replay.New(os.DirFS("../provider/replay/testdata"), speed)
	if err != nil {
		t.Fatalf("replay provider: %v", err)
	}

	return NewPoller(provider, store, pub, Config{
		League:        core.LeagueWNBA,
		ScanInterval:  5 * time.Millisecond,
		WatchInterval: 5 * time.Millisecond,
		MaxInflight:   2,
	}, WithLogger(quietLogger()))
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestPollerRunsAGameToTheEnd(t *testing.T) {
	store := newFakeStore()
	pub := &fakePublisher{}
	poller := newReplayPoller(t, store, pub, 4000)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- poller.Run(ctx) }()

	waitFor(t, "game statistics", func() bool {
		_, _, stats := store.counts()
		return stats == 1
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	games, events, _ := store.counts()
	if games != 1 {
		t.Errorf("want 1 game stored, got %d", games)
	}
	if events == 0 {
		t.Fatal("no events stored")
	}

	published := pub.snapshot()
	if len(published) != events {
		t.Errorf("published %d events but stored %d", len(published), events)
	}

	seen := make(map[core.EventID]bool)
	for _, e := range published {
		if seen[e.ID] {
			t.Errorf("event %s published twice", e.ID)
		}
		seen[e.ID] = true
	}

	kinds := make(map[core.EventKind]bool)
	for _, e := range published {
		kinds[e.Kind] = true
	}
	for _, want := range []core.EventKind{
		core.EventGameStarted, core.EventRun, core.EventLeadChange,
		core.EventCloseFinish, core.EventGameFinal,
	} {
		if !kinds[want] {
			t.Errorf("event %s never published", want)
		}
	}
}

func TestWatcherStopsWhenTheGameIsOver(t *testing.T) {
	store := newFakeStore()
	poller := newReplayPoller(t, store, &fakePublisher{}, 4000)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = poller.Run(ctx) }()

	waitFor(t, "a watcher to start", func() bool { return poller.Watching() > 0 })
	waitFor(t, "the watcher to exit", func() bool { return poller.Watching() == 0 })
}

func TestPollerReturnsPromptlyOnCancel(t *testing.T) {
	poller := newReplayPoller(t, newFakeStore(), &fakePublisher{}, 1)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- poller.Run(ctx) }()

	waitFor(t, "a watcher to start", func() bool { return poller.Watching() > 0 })

	start := time.Now()
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("shutdown took %v", elapsed)
	}
	if poller.Watching() != 0 {
		t.Errorf("%d watchers left behind", poller.Watching())
	}
}

func TestStoreFailureDoesNotKillTheWatcher(t *testing.T) {
	store := newFakeStore()
	store.failNext = errors.New("database is having a moment")
	pub := &fakePublisher{}
	poller := newReplayPoller(t, store, pub, 4000)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = poller.Run(ctx) }()

	waitFor(t, "events despite the failure", func() bool {
		_, events, _ := store.counts()
		return events > 0
	})
}

func TestScheduledGamesAreNotWatchedEarly(t *testing.T) {
	store := newFakeStore()

	provider, err := replay.New(os.DirFS("../provider/replay/testdata"), 1,
		replay.WithStart(time.Now().Add(2*time.Hour)))
	if err != nil {
		t.Fatalf("replay provider: %v", err)
	}

	poller := NewPoller(provider, store, &fakePublisher{}, Config{
		League:       core.LeagueWNBA,
		ScanInterval: 5 * time.Millisecond,
	}, WithLogger(quietLogger()))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = poller.Run(ctx) }()

	waitFor(t, "the scheduled game to be stored", func() bool {
		games, _, _ := store.counts()
		return games == 1
	})

	if n := poller.Watching(); n != 0 {
		t.Errorf("a game starting in two hours got %d watchers", n)
	}
}

func TestFinishRetriesStatsAfterAFailure(t *testing.T) {
	store := newFakeStore()
	store.failStats = 2
	poller := newReplayPoller(t, store, &fakePublisher{}, 4000)
	WithStatsRetry(retry.Policy{Attempts: 4, Base: time.Millisecond, Max: 4 * time.Millisecond})(poller)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = poller.Run(ctx) }()

	waitFor(t, "stats to survive two storage failures", func() bool {
		_, _, stats := store.counts()
		return stats == 1
	})
}
