package postgres

import (
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func seedGame(t *testing.T, store *Store, id core.GameID) core.Game {
	t.Helper()
	seedTeams(t, store)

	g := testGame(id, core.GameLive, time.Now().UTC().Truncate(time.Second), 70, 64)
	if err := store.UpsertGames(t.Context(), []core.Game{g}); err != nil {
		t.Fatalf("seed game: %v", err)
	}
	return g
}

func testEvent(gameID core.GameID, seq int) core.Event {
	return core.Event{
		ID:         core.NewEventID(gameID, core.EventRun, 3, seq),
		GameID:     gameID,
		League:     core.LeagueWNBA,
		Kind:       core.EventRun,
		Teams:      []core.TeamCode{"NYL"},
		Period:     3,
		Clock:      "04:12",
		HomeScore:  70,
		AwayScore:  64,
		Run:        core.RunInfo{Team: "NYL", Points: 15, Against: 0},
		OccurredAt: time.Now().UTC().Truncate(time.Second),
	}
}

func TestSaveEventsIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedGame(t, store, "wnba:1")

	events := []core.Event{testEvent("wnba:1", 142), testEvent("wnba:1", 175)}

	inserted, err := store.SaveEvents(ctx, events)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if len(inserted) != 2 {
		t.Fatalf("want 2 inserted, got %d", len(inserted))
	}
	if inserted[0] != events[0].ID {
		t.Errorf("returned id %q, want %q", inserted[0], events[0].ID)
	}

	inserted, err = store.SaveEvents(ctx, events)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if len(inserted) != 0 {
		t.Errorf("replay inserted %d events, want 0", len(inserted))
	}

	stored, err := store.EventsOfGame(ctx, "wnba:1")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("want 2 stored events, got %d", len(stored))
	}
	if stored[0].Run.Points != 15 || stored[0].Teams[0] != "NYL" {
		t.Errorf("event round-trip lost data: %+v", stored[0])
	}
}

func TestCreateDeliveriesIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedGame(t, store, "wnba:1")

	ev := testEvent("wnba:1", 142)
	if _, err := store.SaveEvents(ctx, []core.Event{ev}); err != nil {
		t.Fatalf("save event: %v", err)
	}

	first, err := store.EnsureSubscriber(ctx, 1)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	second, err := store.EnsureSubscriber(ctx, 2)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	subs := []core.SubscriberID{first.ID, second.ID}

	created, err := store.CreateDeliveries(ctx, ev.ID, subs)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created != 2 {
		t.Fatalf("want 2 deliveries, got %d", created)
	}

	created, err = store.CreateDeliveries(ctx, ev.ID, subs)
	if err != nil {
		t.Fatalf("recreate: %v", err)
	}
	if created != 0 {
		t.Errorf("restart duplicated %d deliveries", created)
	}

	pending, err := store.PendingRecipients(ctx, ev.ID)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("want 2 pending, got %d", len(pending))
	}

	if err := store.MarkDelivery(ctx, first.ID, ev.ID, core.DeliverySent); err != nil {
		t.Fatalf("mark: %v", err)
	}

	pending, err = store.PendingRecipients(ctx, ev.ID)
	if err != nil {
		t.Fatalf("pending after mark: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != second.ID {
		t.Errorf("unexpected pending set: %+v", pending)
	}
}

func TestEventsNeedingDelivery(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()
	seedGame(t, store, "wnba:1")

	sent := testEvent("wnba:1", 142)
	stuck := testEvent("wnba:1", 175)
	orphan := testEvent("wnba:1", 201)
	if _, err := store.SaveEvents(ctx, []core.Event{sent, stuck, orphan}); err != nil {
		t.Fatalf("save: %v", err)
	}

	sub, err := store.EnsureSubscriber(ctx, 1)
	if err != nil {
		t.Fatalf("subscriber: %v", err)
	}
	for _, ev := range []core.Event{sent, stuck} {
		if _, err := store.CreateDeliveries(ctx, ev.ID, []core.SubscriberID{sub.ID}); err != nil {
			t.Fatalf("deliveries: %v", err)
		}
	}
	if err := store.MarkDelivery(ctx, sub.ID, sent.ID, core.DeliverySent); err != nil {
		t.Fatalf("mark: %v", err)
	}

	got, err := store.EventsNeedingDelivery(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	ids := make(map[core.EventID]bool, len(got))
	for _, e := range got {
		ids[e.ID] = true
	}
	if len(got) != 2 || !ids[stuck.ID] || !ids[orphan.ID] {
		t.Fatalf("want the stuck and the orphan events, got %+v", ids)
	}

	none, err := store.EventsNeedingDelivery(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("future window: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("window ignored, got %d events", len(none))
	}
}
