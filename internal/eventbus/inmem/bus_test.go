package inmem

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/damnary/nba-digest/internal/core"
)

func event(id core.EventID) core.Event {
	return core.Event{ID: id, Kind: core.EventRun, League: core.LeagueWNBA}
}

func TestEventsReachTheConsumerInOrder(t *testing.T) {
	bus := New(8)

	var (
		mu   sync.Mutex
		seen []core.EventID
		wg   sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = bus.Consume(t.Context(), func(_ context.Context, e core.Event) error {
			mu.Lock()
			seen = append(seen, e.ID)
			mu.Unlock()
			return nil
		})
	}()

	for _, id := range []core.EventID{"a", "b", "c"} {
		if err := bus.Publish(t.Context(), event(id)); err != nil {
			t.Fatalf("publish %s: %v", id, err)
		}
	}
	bus.Close()
	wg.Wait()

	if len(seen) != 3 || seen[0] != "a" || seen[2] != "c" {
		t.Errorf("consumer saw %v", seen)
	}
}

func TestConsumerStopsWhenTheBusIsClosed(t *testing.T) {
	bus := New(1)
	done := make(chan error, 1)

	go func() {
		done <- bus.Consume(t.Context(), func(context.Context, core.Event) error { return nil })
	}()

	bus.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Consume returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop after the bus was closed")
	}
}

func TestCloseIsSafeTwice(t *testing.T) {
	bus := New(1)
	bus.Close()
	bus.Close()
}

func TestPublishBlocksWhenFullAndRespectsContext(t *testing.T) {
	bus := New(1)

	if err := bus.Publish(t.Context(), event("first")); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if bus.Len() != 1 {
		t.Fatalf("buffer should hold one event, got %d", bus.Len())
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err := bus.Publish(ctx, event("second"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a full buffer must block until the context expires, got %v", err)
	}
}

func TestHandlerFailuresAreCounted(t *testing.T) {
	bus := New(4)

	go func() {
		_ = bus.Publish(context.Background(), event("a"))
		_ = bus.Publish(context.Background(), event("b"))
		bus.Close()
	}()

	err := bus.Consume(t.Context(), func(context.Context, core.Event) error {
		return errors.New("handler is unhappy")
	})
	if err != nil {
		t.Fatalf("Consume returned %v", err)
	}
	if bus.Dropped() != 2 {
		t.Errorf("dropped = %d, want 2", bus.Dropped())
	}
}
