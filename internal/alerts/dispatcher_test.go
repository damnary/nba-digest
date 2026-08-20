package alerts

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
	"github.com/damnary/nba-digest/internal/eventbus/inmem"
)

var now = time.Date(2026, 8, 11, 3, 30, 0, 0, time.UTC)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type delivery struct {
	sub    core.SubscriberID
	event  core.EventID
	status core.DeliveryStatus
}

type fakeStore struct {
	subs      []core.Subscriber
	pending   map[core.EventID][]core.Subscriber
	created   map[core.EventID][]core.SubscriberID
	marks     []delivery
	catchUp   []core.Event
	failFind  bool
	failCatch bool
}

func newFakeStore(subs ...core.Subscriber) *fakeStore {
	return &fakeStore{
		subs:    subs,
		pending: make(map[core.EventID][]core.Subscriber),
		created: make(map[core.EventID][]core.SubscriberID),
	}
}

func (f *fakeStore) SubscribersForTeams(context.Context, core.League, []core.TeamCode) ([]core.Subscriber, error) {
	if f.failFind {
		return nil, errors.New("database is down")
	}
	return f.subs, nil
}

func (f *fakeStore) CreateDeliveries(_ context.Context, id core.EventID, subs []core.SubscriberID) (int, error) {
	existing := make(map[core.SubscriberID]bool)
	for _, s := range f.created[id] {
		existing[s] = true
	}

	created := 0
	for _, s := range subs {
		if existing[s] {
			continue
		}
		f.created[id] = append(f.created[id], s)
		created++

		for _, sub := range f.subs {
			if sub.ID == s {
				f.pending[id] = append(f.pending[id], sub)
			}
		}
	}
	return created, nil
}

func (f *fakeStore) PendingRecipients(_ context.Context, id core.EventID) ([]core.Subscriber, error) {
	out := make([]core.Subscriber, 0, len(f.pending[id]))
	for _, pending := range f.pending[id] {
		for _, current := range f.subs {
			if current.ID == pending.ID {
				out = append(out, current)
			}
		}
	}
	return out, nil
}

func (f *fakeStore) MarkDelivery(_ context.Context, id core.SubscriberID, event core.EventID, status core.DeliveryStatus) error {
	f.marks = append(f.marks, delivery{sub: id, event: event, status: status})

	if status != core.DeliveryPending {
		var kept []core.Subscriber
		for _, sub := range f.pending[event] {
			if sub.ID != id {
				kept = append(kept, sub)
			}
		}
		f.pending[event] = kept
	}
	return nil
}

func (f *fakeStore) EventsNeedingDelivery(context.Context, time.Time) ([]core.Event, error) {
	if f.failCatch {
		return nil, errors.New("database is down")
	}
	return f.catchUp, nil
}

type fakeSender struct {
	sent []int64
	fail bool
}

func (s *fakeSender) SendEvent(ctx context.Context, chatID int64, _ core.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.fail {
		return errors.New("telegram said no")
	}
	s.sent = append(s.sent, chatID)
	return nil
}

type fakeConsumer struct {
	events []core.Event
}

func (c *fakeConsumer) Consume(ctx context.Context, handle func(context.Context, core.Event) error) error {
	for _, e := range c.events {
		if err := handle(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func subscriber(id core.SubscriberID, chat int64, alerts bool) core.Subscriber {
	return core.Subscriber{ID: id, ChatID: chat, AlertsOn: alerts, Timezone: time.UTC}
}

func testEvent() core.Event {
	return core.Event{
		ID:         "wnba:1:run:p3:000142",
		GameID:     "wnba:1",
		League:     core.LeagueWNBA,
		Kind:       core.EventRun,
		Teams:      []core.TeamCode{"NYL", "LVA"},
		Run:        core.RunInfo{Team: "NYL", Points: 12, Against: 2},
		OccurredAt: now.Add(-time.Minute),
	}
}

func newDispatcher(store Store, sender Sender, consumer Consumer) *Dispatcher {
	return New(store, sender, consumer,
		WithLogger(quiet()),
		WithClock(func() time.Time { return now }))
}

func TestDispatchSendsToSubscribersWithAlertsOn(t *testing.T) {
	store := newFakeStore(
		subscriber(1, 100, true),
		subscriber(2, 200, false),
	)
	sender := &fakeSender{}
	d := newDispatcher(store, sender, &fakeConsumer{})

	if err := d.Dispatch(t.Context(), testEvent()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(sender.sent) != 1 || sender.sent[0] != 100 {
		t.Errorf("sent to %v, want only chat 100", sender.sent)
	}
	if len(store.marks) != 1 || store.marks[0].status != core.DeliverySent {
		t.Errorf("marks = %+v", store.marks)
	}
}

func TestDispatchIsIdempotent(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	sender := &fakeSender{}
	d := newDispatcher(store, sender, &fakeConsumer{})
	event := testEvent()

	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}

	if len(sender.sent) != 1 {
		t.Errorf("the same event was delivered %d times", len(sender.sent))
	}
}

func TestStaleEventIsSkippedNotSent(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	sender := &fakeSender{}
	d := newDispatcher(store, sender, &fakeConsumer{})

	event := testEvent()
	event.OccurredAt = now.Add(-2 * core.CatchUpWindow)

	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(sender.sent) != 0 {
		t.Errorf("an hour-old event must not be sent, got %v", sender.sent)
	}
	if len(store.marks) != 1 || store.marks[0].status != core.DeliverySkipped {
		t.Errorf("marks = %+v, want one skipped", store.marks)
	}
}

func TestSendFailureIsRecordedAndDoesNotStopTheRest(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true), subscriber(2, 200, true))
	sender := &fakeSender{fail: true}
	d := newDispatcher(store, sender, &fakeConsumer{})

	if err := d.Dispatch(t.Context(), testEvent()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	if len(store.marks) != 2 {
		t.Fatalf("both recipients should be marked, got %+v", store.marks)
	}
	for _, m := range store.marks {
		if m.status != core.DeliveryPending {
			t.Errorf("status = %s, want pending so the sweep can retry", m.status)
		}
	}
}

func TestNobodySubscribedMeansNothingSent(t *testing.T) {
	store := newFakeStore()
	sender := &fakeSender{}
	d := newDispatcher(store, sender, &fakeConsumer{})

	if err := d.Dispatch(t.Context(), testEvent()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(sender.sent) != 0 || len(store.marks) != 0 {
		t.Errorf("sent %v, marks %+v", sender.sent, store.marks)
	}
}

func TestRunCatchesUpThenConsumes(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))

	missed := testEvent()
	missed.ID = "wnba:1:run:p2:000050"
	store.catchUp = []core.Event{missed}

	live := testEvent()
	sender := &fakeSender{}
	d := newDispatcher(store, sender, &fakeConsumer{events: []core.Event{live}})

	if err := d.Run(t.Context()); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(sender.sent) != 2 {
		t.Fatalf("want the missed event and the live one, got %d sends", len(sender.sent))
	}
	if store.marks[0].event != missed.ID {
		t.Errorf("catch-up should come first, got %s", store.marks[0].event)
	}
}

func TestStoreFailureIsReported(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	store.failFind = true
	d := newDispatcher(store, &fakeSender{}, &fakeConsumer{})

	if err := d.Dispatch(t.Context(), testEvent()); err == nil {
		t.Fatal("want an error when the store fails")
	}
}

func TestFailedSendStaysRetryable(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	sender := &fakeSender{fail: true}
	d := newDispatcher(store, sender, &fakeConsumer{})
	event := testEvent()

	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(store.marks) != 1 || store.marks[0].status != core.DeliveryPending {
		t.Fatalf("a failed send must stay pending, got %+v", store.marks)
	}

	sender.fail = false
	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0] != 100 {
		t.Errorf("the retry should have delivered the alert, got %v", sender.sent)
	}
}

func TestSweepRetriesPendingDeliveries(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	sender := &fakeSender{fail: true}
	d := newDispatcher(store, sender, &fakeConsumer{})

	store.catchUp = []core.Event{testEvent()}

	if err := d.catchUp(t.Context()); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("telegram was down, nothing should have been delivered")
	}

	sender.fail = false
	if err := d.catchUp(t.Context()); err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0] != 100 {
		t.Errorf("the sweep should have delivered the alert, got %v", sender.sent)
	}
}

func TestAlertsDisabledAfterCreationAreSkippedAtSendTime(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	sender := &fakeSender{fail: true}
	d := newDispatcher(store, sender, &fakeConsumer{})
	event := testEvent()

	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	store.subs[0].AlertsOn = false
	sender.fail = false
	if err := d.Dispatch(t.Context(), event); err != nil {
		t.Fatalf("retry: %v", err)
	}

	if len(sender.sent) != 0 {
		t.Errorf("alerts were turned off, nothing should be sent: %v", sender.sent)
	}
	last := store.marks[len(store.marks)-1]
	if last.status != core.DeliverySkipped {
		t.Errorf("final status = %s, want skipped", last.status)
	}
}

func TestShutdownDrainsTheBuffer(t *testing.T) {
	store := newFakeStore(subscriber(1, 100, true))
	sender := &fakeSender{}

	bus := inmem.New(8)
	first := testEvent()
	second := testEvent()
	second.ID = "wnba:1:run:p3:000200"
	if err := bus.Publish(t.Context(), first); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := bus.Publish(t.Context(), second); err != nil {
		t.Fatalf("publish: %v", err)
	}
	bus.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d := New(store, sender, bus,
		WithLogger(quiet()),
		WithClock(func() time.Time { return now }),
		WithDrainGrace(5*time.Second))

	if err := d.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(sender.sent) != 2 {
		t.Errorf("the buffer must be drained after shutdown, sent %d of 2", len(sender.sent))
	}
	if bus.Dropped() != 0 {
		t.Errorf("%d events were dropped instead of drained", bus.Dropped())
	}
}
